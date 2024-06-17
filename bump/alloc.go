package bump

import (
	"math/bits"
	"unsafe"
)

const (
	BlockSize  = 8 << 10
	lineSize   = 128
	headerSize = unsafe.Sizeof(uint64(0))
	minAlign   = 8
)

type Allocator struct {
	main     *Block
	overflow *Block
	full     *Block
	existing []*Block
}

func NewAllocator(blocks []*Block) *Allocator {
	return &Allocator{existing: blocks}
}

func (a *Allocator) Make(size uintptr, header uint64) unsafe.Pointer {
	if a.main == nil {
		a.main = a.getBlock()
	}
	fullSize := alignUp(size+headerSize, minAlign)
	var addr unsafe.Pointer
outerLoop:
	for {
		if addr = a.main.tryAlloc(fullSize); addr != nil {
			break
		}
		if fullSize > lineSize && a.main.limit-a.main.cursor > lineSize {
			if a.overflow == nil {
				a.overflow = NewBlock(0)
			}
			for {
				if addr = a.overflow.tryAlloc(fullSize); addr != nil {
					break outerLoop
				}
				a.overflow = NewBlock(0)
			}
		}
		a.main = a.getBlock()
	}
	*(*uint64)(addr) = header
	memclrNoHeapPointers(unsafe.Add(addr, headerSize), size)
	return addr
}

func (a *Allocator) getBlock() *Block {
	if len(a.existing) == 0 {
		return NewBlock(0)
	}
	b := a.existing[0]
	a.existing = a.existing[1:]
	return b
}

type Block struct {
	cursor, limit uintptr
	lineAlloc     uint64
	lineMark      uint64
	next          *Block
	data          *[BlockSize]byte
}

func NewBlock(lines uint64) *Block {
	blk := new(Block)
	blk.lineMark = lines
	blk.data = new([BlockSize]byte)
	blk.Reset()
	return blk
}

func (b *Block) tryAlloc(size uintptr) unsafe.Pointer {
	if addr := b.tryAllocFast(size); addr != nil {
		return addr
	}
	return b.tryAlloc1(size)
}

func (b *Block) tryAllocFast(size uintptr) unsafe.Pointer {
	c := b.cursor
	n := c + size
	if n < b.limit {
		b.cursor = n
		wi := (c - uintptr(unsafe.Pointer(b.data))) / minAlign
		b.data[128+wi/8] |= 1 << (wi % 8)
		return unsafe.Pointer(c)
	}
	return nil
}

func (b *Block) tryAlloc1(size uintptr) unsafe.Pointer {
	for {
		if !b.refill() {
			return nil
		}
		if addr := b.tryAllocFast(size); addr != nil {
			return addr
		}
	}
}

func (b *Block) refill() bool {
	lineAlloc := b.lineAlloc
	i := bits.TrailingZeros64(^lineAlloc)
	if i == 64 {
		return false
	}
	n := bits.TrailingZeros64(lineAlloc >> i)
	if n == 64 {
		n -= i
	}
	b.lineAlloc = lineAlloc | (((1 << n) - 1) << i)
	b.cursor = uintptr(unsafe.Pointer(b.data)) + uintptr(i)*lineSize
	b.limit = b.cursor + uintptr(n)*lineSize
	return true
}

func (b *Block) Reset() {
	b.lineAlloc = b.lineMark | 0b11
}

func alignUp(x, align uintptr) uintptr {
	return (x + align - 1) &^ (align - 1)
}

//go:linkname memclrNoHeapPointers runtime.memclrNoHeapPointers
func memclrNoHeapPointers(addr unsafe.Pointer, size uintptr)

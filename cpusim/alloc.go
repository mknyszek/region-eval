// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpusim

import (
	"math/bits"
	"unsafe"

	"github.com/mknyszek/region-eval/cpusim/bitmath"
)

const (
	BlockSize  = 8 << 10
	lineSize   = 128
	headerSize = unsafe.Sizeof(uint64(0))
	minAlign   = 8
	BitmapSize = BlockSize / minAlign / 8
)

func init() {
	if BitmapSize != lineSize {
		panic("each block bitmap must fit exactly in one line")
	}
}

type Pointer unsafe.Pointer

type Allocator struct {
	main     *Block
	overflow *Block
	full     *Block
	existing []*Block
}

func NewAllocator(blocks []*Block) *Allocator {
	return &Allocator{existing: blocks}
}

func (a *Allocator) Make(size uintptr, typ *FakeType) Pointer {
	if a.main == nil {
		a.main = a.getBlock()
	}
	fullSize := size
	fullSize += headerSize
	fullSize = bitmath.AlignUp(fullSize, minAlign)
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
		a.main.next = a.full
		a.full = a.main
		a.main = a.getBlock()
	}
	*(*uint64)(addr) = uint64(uintptr(unsafe.Pointer(typ))) | (uint64(size/8) << 48)
	memclrNoHeapPointers(unsafe.Add(addr, headerSize), size)
	return Pointer(unsafe.Add(addr, headerSize))
}

func (a *Allocator) Reset() {
	for a.full != nil {
		a.full.Reset()
		a.existing = append(a.existing, a.full)
		a.full = a.full.next
	}
}

func (a *Allocator) getBlock() *Block {
	n := len(a.existing)
	if n == 0 {
		return NewBlock(0)
	}
	b := a.existing[n-1]
	a.existing = a.existing[:n-1]
	return b
}

func (a *Allocator) BlockOf(ptr Pointer) *Block {
	if a.main.Contains(ptr) {
		return a.main
	}
	if a.overflow.Contains(ptr) {
		return a.overflow
	}
	f := a.full
	for f != nil {
		if f.Contains(ptr) {
			return f
		}
		f = f.next
	}
	return nil
}

type Block struct {
	cursor, limit uintptr
	lineAlloc     uint64
	next          *Block
	data          *[BlockSize]byte
}

func NewBlock(lines uint64) *Block {
	blk := new(Block)
	blk.data = new([BlockSize]byte)
	d := (*BlockMeta)(unsafe.Pointer(&blk.data[0]))
	d.LineEscape = lines
	blk.Reset()
	return blk
}

func NewBlockFromExisting(lines uint64, region uintptr, data *[BlockSize]byte) *Block {
	blk := new(Block)
	blk.data = data
	d := (*BlockMeta)(unsafe.Pointer(&blk.data[0]))
	d.LineEscape = lines
	d.Region = region
	blk.Reset()
	return blk
}

func (b *Block) Contains(ptr Pointer) bool {
	s := uintptr(unsafe.Pointer(&b.data[0]))
	e := uintptr(unsafe.Pointer(&b.data[len(b.data)-1]))
	p := uintptr(ptr)
	return s <= p && p <= e
}

func (b *Block) Base() uintptr {
	return uintptr(unsafe.Pointer(&b.data[0]))
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
		wi := (c - b.Base()) / minAlign
		b.data[BitmapSize+wi/8] |= 1 << (wi % 8)
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
	if i == 2 {
		b.cursor += 16 // Room for LineEscape and Region.
	}
	return true
}

func (b *Block) Reset() {
	// First two lines are reserved.
	d := b.Meta()
	b.lineAlloc = d.LineEscape | 0b11
	b.cursor, b.limit = 0, 0

	// Clear ObjBits.

	// Make the math easier by reinterpreting ObjBits as 16-bit chunks.
	// Only works on little-endian.
	ObjBits := (*[BitmapSize / 2]uint16)(unsafe.Pointer(&d.ObjBits[0]))

	clearIter := b.lineAlloc
	for clearIter != 0 {
		i := bits.TrailingZeros64(^clearIter)
		clearIter >>= i
		n := bits.TrailingZeros64(clearIter)
		if n == 64 {
			n -= i
		}
		toClear := ObjBits[i : i+n]
		for i := range toClear {
			toClear[i] = 0
		}
		clearIter >>= n
	}
}

func (b *Block) Meta() *BlockMeta {
	return (*BlockMeta)(unsafe.Pointer(&b.data[0]))
}

type BlockMeta struct {
	EscBits    [BitmapSize / 8]uint64
	ObjBits    [BitmapSize / 8]uint64
	LineEscape uint64
	Region     uintptr
}

//go:linkname memclrNoHeapPointers runtime.memclrNoHeapPointers
func memclrNoHeapPointers(addr unsafe.Pointer, size uintptr)

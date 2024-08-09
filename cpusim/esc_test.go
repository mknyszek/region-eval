// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpusim_test

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/aclements/go-perfevent/perfbench"
	"github.com/mknyszek/region-eval/cpusim"
	"github.com/mknyszek/region-eval/cpusim/bitmath"
)

func BenchmarkEscape(b *testing.B) {
	for _, ptrPercent := range []int{0, 25, 50, 75, 100} {
		b.Run(fmt.Sprintf("percentPointers=%d", ptrPercent), func(b *testing.B) {
			// We use a ballast rather than SetMemoryLimit to get a typical GC sawtooth
			// so that we in turn get "typical" reuse of swept memory. It *probably*
			// doesn't matter, but this way we don't have to worry about triggering
			// strange behavior from a near-empty post-GC heap.
			ballast = make([]byte, llcBytes)
			defer func() { ballast = nil }()

			if ptrPercent == 0 || ptrPercent == 100 {
				benchEscape(b, 8, ptrPercent)
			}
			if ptrPercent == 0 || ptrPercent == 50 || ptrPercent == 100 {
				benchEscape(b, 16, ptrPercent)
			}
			benchEscape(b, 32, ptrPercent)
			benchEscape(b, 64, ptrPercent)
			benchEscape(b, 128, ptrPercent)
			benchEscape(b, 256, ptrPercent)
			benchEscape(b, 512, ptrPercent)
			benchEscape(b, 1024, ptrPercent)
			benchEscape(b, 2048, ptrPercent)
		})
	}
}

func benchEscape(b *testing.B, size uintptr, ptrPercent int) {
	b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
		cs := perfbench.Open(b)

		a := cpusim.NewAllocator(nil)
		ft := makeFakeType(size, ptrPercent)

		// Allocate a whole bunch of things to escape, up to half the ballast.
		escapes := make([]cpusim.Pointer, 0, 2*len(ballast)/int(size))
		var total uintptr
		for {
			x := a.Make(size, ft)
			if alwaysFalse {
				sink = x
			}
			escapes = append(escapes, x)
			total += 8 + size
			if total > uintptr(len(ballast)/2) {
				break
			}
		}

		// Shuffle up the pointers so we get plenty of cache misses.
		r := rand.New(rand.NewPCG(0, 0))
		r.Shuffle(len(escapes), func(i, j int) {
			escapes[i], escapes[j] = escapes[j], escapes[i]
		})

		// Run a GC now to avoid having one trigger later from some small allocation.
		runtime.GC()

		var mstats runtime.MemStats
		runtime.ReadMemStats(&mstats)
		startGCs := mstats.NumGC

		b.ResetTimer()
		cs.Reset()

		for i := range b.N {
			cpusim.MarkEscaped(escapes[i%len(escapes)])
		}

		cs.Stop()
		b.StopTimer()

		reportPerByte(b, size, cs)

		// Confirm that no automatic GCs happened during the benchmark.
		runtime.ReadMemStats(&mstats)
		endGCs := mstats.NumGC
		if endGCs != startGCs {
			b.Fatalf("%d unaccounted GCs", endGCs-startGCs)
		}
	})
}

func makeFakeType(size uintptr, ptrPercent int) *cpusim.FakeType {
	gcdata := []byte(nil)
	ptrBytes := uintptr(0)
	if ptrPercent != 0 {
		ptrBytes = bitmath.AlignDown(size, 8)
		gcdata = make([]byte, bitmath.AlignUp(bitmath.AlignUp(ptrBytes/8, 8)/8, 8))
		nwords := size / 8
		invert := ptrPercent > 50 && ptrPercent != 100
		if invert {
			ptrPercent = 100 - ptrPercent
		}
		nptrs := nwords * uintptr(ptrPercent) / 100
		spacing := nwords / nptrs
		for i := uintptr(0); i < size; i += 8 {
			j := i / 8
			if (!invert && j%spacing == 0) || (invert && j%spacing != 0) {
				gcdata[j/8] |= uint8(1) << (j % 8)
			}
		}
	}
	return cpusim.NewFakeType(size, ptrBytes, gcdata)
}

func TestMarkEscaped(t *testing.T) {
	for _, size := range []uintptr{8, 16, 24, 32, 64, 248, 256, 512, 1024, 2048} {
		for offset := uintptr(0); offset < size; offset++ {
			for _, ptrs := range []bool{false, true} {
				t.Run(fmt.Sprintf("size=%d/ptrs=%t/offset=%d", size, ptrs, offset), func(t *testing.T) {
					testMarkEscaped(t, size, offset, ptrs)
				})
			}
		}
	}
}

func testMarkEscaped(t *testing.T, size, offset uintptr, ptrs bool) {
	a := cpusim.NewAllocator(nil)
	ppct := 0
	if ptrs {
		ppct = 100
	}
	ft := makeFakeType(size, ppct)
	var x cpusim.Pointer
	for i := 0; i < 7; i++ {
		x = a.Make(size, ft)
		if alwaysFalse {
			sink = x
		}
	}
	b := a.BlockOf(x)
	d := b.Meta()
	ws := (uintptr(x)-b.Base())/8 - 1
	we := (uintptr(x) + size - b.Base()) / 8

	// Check that the before state makes sense.
	if !isSet(&d.ObjBits, ws) {
		t.Fatal("start bit not set for object")
	}
	for i := ws + 1; i < we; i++ {
		if isSet(&d.ObjBits, i) {
			t.Fatal("found non-start bit to be set")
		}
	}
	for i := uintptr(0); i < uintptr(len(d.EscBits)*64); i++ {
		if isSet(&d.EscBits, i) {
			t.Fatal("found escape bit to be set before mark")
		}
	}

	// Update the escaped status.
	cpusim.MarkEscaped(cpusim.Pointer(uintptr(x) + offset))

	// Validate that the update happened correctly.
	if !isSet(&d.ObjBits, ws) {
		t.Fatal("start bit not set for object")
	}
	for i := ws + 1; i < we; i++ {
		if isSet(&d.ObjBits, i) {
			t.Fatal("found non-start bit to be set")
		}
	}
	for i := uintptr(0); i < ws; i++ {
		if isSet(&d.EscBits, i) {
			t.Fatalf("found escape bit %d incorrectly set", i)
		}
	}
	for i := ws; i < we; i++ {
		if !isSet(&d.EscBits, i) {
			t.Fatalf("found escape bit %d not to be set", i)
		}
	}
	for i := we; i < uintptr(len(d.EscBits)*64); i++ {
		if isSet(&d.EscBits, i) {
			t.Fatalf("found escape bit %d incorrectly set", i)
		}
	}
}

func isSet(b *[cpusim.BitmapSize / 8]uint64, i uintptr) bool {
	return b[i/64]&(uint64(1)<<(i%64)) != 0
}

func BenchmarkWriteBarrier(b *testing.B) {
	for _, shuffle := range []bool{false, true} {
		for _, preEscPercent := range []int{0, 1, 10, 50, 100} {
			b.Run(fmt.Sprintf("shuffle=%t/percentPreEscaped=%d", shuffle, preEscPercent), func(b *testing.B) {
				benchWriteBarrier(b, preEscPercent, shuffle)
			})
		}
	}
}

func benchWriteBarrier(b *testing.B, preEscPercent int, shuffle bool) {
	cs := perfbench.Open(b)

	dataSize := 1 << 30
	data, err := syscall.Mmap(-1, 0, dataSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		b.Fatal(err)
	}
	addr := uintptr(unsafe.Pointer(&data[0]))
	for i := addr; i < addr+uintptr(dataSize)+cpusim.HeapArenaBytes; i += cpusim.HeapArenaBytes {
		arenaIdx := i / cpusim.HeapArenaBytes
		cpusim.IsRegionArena[arenaIdx/64] |= uint64(1) << (arenaIdx % 64)
	}
	defer func() {
		syscall.Munmap(data)
		for i := range cpusim.IsRegionArena {
			cpusim.IsRegionArena[i] = 0
		}
	}()

	// The cpusim code assumes BlockSize-aligned memory, but mmap may not return memory with sufficient alignment.
	// Since we have plenty of blocks, align it up ourselves.
	alignedData := data
	if bitmath.AlignDown(addr, cpusim.BlockSize) != addr {
		offset := bitmath.AlignUp(addr, cpusim.BlockSize) - addr
		alignedData = data[offset:]
	}

	// Split the mmap'd data into blocks.
	var blocks []*cpusim.Block
	for i := 0; i < len(alignedData); i += cpusim.BlockSize {
		if len(alignedData[i:]) < cpusim.BlockSize {
			break
		}
		blocks = append(blocks, cpusim.NewBlockFromExisting(0, 0, (*[cpusim.BlockSize]byte)(alignedData[i:i+cpusim.BlockSize])))
	}

	const fp = 64 << 10
	const sz = 64
	const n = fp / sz
	size := uintptr(sz) - 8 // Total size is 64 for each alloc.
	a := cpusim.NewAllocator(blocks)
	ft := makeFakeType(size, 100)

	// Allocate a whole bunch of things to escape.
	r := rand.New(rand.NewPCG(0, 0))
	escapes := make([]unsafe.Pointer, 0, 2*n)
	for range cap(escapes) {
		x := a.Make(size, ft)
		if alwaysFalse {
			sink = x
		}
		if preEscPercent != 0 && r.IntN(100/preEscPercent) == 0 {
			cpusim.MarkEscaped(x)
		}
		escapes = append(escapes, unsafe.Pointer(x))
	}

	// Shuffle up the pointers so we get plenty of cache misses.
	if shuffle {
		r.Shuffle(len(escapes), func(i, j int) {
			escapes[i], escapes[j] = escapes[j], escapes[i]
		})
	}
	srcs := make([]unsafe.Pointer, 0, n)
	dsts := make([]unsafe.Pointer, 0, n)
	for i, x := range escapes {
		if i%2 == 0 {
			srcs = append(srcs, x)
		} else {
			dsts = append(dsts, x)
		}
	}

	// Run a GC now to avoid having one trigger later from some small allocation.
	runtime.GC()

	var mstats runtime.MemStats
	runtime.ReadMemStats(&mstats)
	startGCs := mstats.NumGC

	b.ResetTimer()
	cs.Reset()

	for i := range b.N {
		ptr, dst := srcs[i%n], dsts[i%n]
		cpusim.RegionWriteBarrierFastPath(ptr, dst)
		*(*uintptr)(dst) = uintptr(ptr)
	}

	cs.Stop()
	b.StopTimer()

	reportPerByte(b, size, cs)

	// Confirm that no automatic GCs happened during the benchmark.
	runtime.ReadMemStats(&mstats)
	endGCs := mstats.NumGC
	if endGCs != startGCs {
		b.Fatalf("%d unaccounted GCs", endGCs-startGCs)
	}
}

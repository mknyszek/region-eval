// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpusim_test

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"testing"

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

// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bump_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/aclements/go-perfevent/perfbench"
	"github.com/mknyszek/region-eval/bump"
)

const llcBytes = 16 << 20 // LLC size or larger

var ballast []byte
var sink any
var alwaysFalse bool

func BenchmarkAlloc(b *testing.B) {
	for _, ptrs := range []bool{false, true} {
		for _, reset := range []bool{false, true} {
			b.Run(fmt.Sprintf("ptrs=%t/reset=%t", ptrs, reset), func(b *testing.B) {
				// We use a ballast rather than SetMemoryLimit to get a typical GC sawtooth
				// so that we in turn get "typical" reuse of swept memory. It *probably*
				// doesn't matter, but this way we don't have to worry about triggering
				// strange behavior from a near-empty post-GC heap.
				ballast = make([]byte, llcBytes)
				defer func() { ballast = nil }()

				bench(b, 8, ptrs, reset)
				bench(b, 16, ptrs, reset)
				bench(b, 32, ptrs, reset)
				bench(b, 64, ptrs, reset)
				bench(b, 128, ptrs, reset)
				bench(b, 256, ptrs, reset)
				bench(b, 512, ptrs, reset)
				bench(b, 1024, ptrs, reset)
				bench(b, 2048, ptrs, reset)
			})
		}
	}
}

func bench(b *testing.B, size uintptr, ptrs, benchReset bool) {
	b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
		cs := perfbench.Open(b)

		var mstats runtime.MemStats
		runtime.ReadMemStats(&mstats)
		startGCs := mstats.NumGC

		a := bump.NewAllocator(nil)

		b.ResetTimer()
		cs.Reset()

		ptrBytes := uintptr(0)
		if ptrs {
			ptrBytes = 8
		}
		ft := bump.NewFakeType(size, ptrBytes)

		var total uintptr
		for range b.N {
			x := a.Make(size, ft)
			if alwaysFalse {
				sink = x
			}
			total += 8 + size
			if total > uintptr(len(ballast)/2) {
				if benchReset {
					// Reset the allocator as part of the benchmark.
					a.Reset()
				}

				// Run GC manually so we can exclude GC time from the benchmark results.
				cs.Stop()
				b.StopTimer()
				if !benchReset {
					// Reset the allocator now.
					a.Reset()
				}
				total = 0
				runtime.GC()
				startGCs++
				b.StartTimer()
				cs.Start()
			}
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

func reportPerByte(b *testing.B, bytesPerOp uintptr, cs *perfbench.Counters) {
	bytes := bytesPerOp * uintptr(b.N)
	duration := b.Elapsed()
	b.ReportMetric(float64(duration.Nanoseconds())/float64(bytes), "ns/byte")
	if cycles, ok := cs.Total("cpu-cycles"); ok {
		b.ReportMetric(cycles/float64(bytes), "cpu-cycles/byte")
	}
	if inst, ok := cs.Total("instructions"); ok {
		b.ReportMetric(inst/float64(bytes), "instructions/byte")
	}
}

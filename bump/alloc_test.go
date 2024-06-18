package bump_test

import (
	"fmt"
	"math/rand/v2"
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
	// We use a ballast rather than SetMemoryLimit to get a typical GC sawtooth
	// so that we in turn get "typical" reuse of swept memory. It *probably*
	// doesn't matter, but this way we don't have to worry about triggering
	// strange behavior from a near-empty post-GC heap.
	ballast = make([]byte, llcBytes)
	defer func() { ballast = nil }()

	bench(b, 8, false)
	bench(b, 16, false)
	bench(b, 32, false)
	bench(b, 64, false)
	bench(b, 128, false)
	bench(b, 256, false)
	bench(b, 512, false)
	bench(b, 1024, false)
	bench(b, 2048, false)
}

func BenchmarkAllocAndReset(b *testing.B) {
	ballast = make([]byte, llcBytes)
	defer func() { ballast = nil }()

	bench(b, 8, true)
	bench(b, 16, true)
	bench(b, 32, true)
	bench(b, 64, true)
	bench(b, 128, true)
	bench(b, 256, true)
	bench(b, 512, true)
	bench(b, 1024, true)
	bench(b, 2048, true)
}

func bench(b *testing.B, size uintptr, benchReset bool) {
	header := rand.Uint64()

	b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
		cs := perfbench.Open(b)

		var mstats runtime.MemStats
		runtime.ReadMemStats(&mstats)
		startGCs := mstats.NumGC

		a := bump.NewAllocator(nil)

		b.ResetTimer()
		cs.Reset()

		var total uintptr
		for range b.N {
			x := a.Make(size, header)
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

		//reportPerByte(b, size, cs)

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

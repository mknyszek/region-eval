// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "time"

var Scenarios = []Scenario{
	{
		Name:                        "IdealBroadUse",
		RegionAllocBytesFrac:        0.6,
		RegionAllocsFrac:            0.9,
		FadeAllocBytesFrac:          0.05,
		FadeAllocsFrac:              0.05,
		ScannedRegionAllocBytesFrac: 0.01,
		RegionScanCostRatio:         1.05,
		FadeAllocsPointerDensity:    0.125 / 2,
	},
	{
		Name:                        "BestPossible",
		RegionAllocBytesFrac:        1.0,
		RegionAllocsFrac:            1.0,
		FadeAllocBytesFrac:          0.0,
		FadeAllocsFrac:              0.0,
		ScannedRegionAllocBytesFrac: 0.0,
		RegionScanCostRatio:         1.0,
		FadeAllocsPointerDensity:    0.0,
	},
	{
		Name:                        "WorstPossible",
		RegionAllocBytesFrac:        1.0,
		RegionAllocsFrac:            1.0,
		FadeAllocBytesFrac:          1.0,
		FadeAllocsFrac:              1.0,
		ScannedRegionAllocBytesFrac: 0.0,
		RegionScanCostRatio:         1.05,
		FadeAllocsPointerDensity:    0.125,
	},
}

type Scenario struct {
	Name                        string
	RegionAllocBytesFrac        float64 // Fraction of bytes that are allocated in a region.
	RegionAllocsFrac            float64 // Fraction of objects that are allocated in a region.
	FadeAllocBytesFrac          float64 // Fraction of region-allocated bytes that fade.
	FadeAllocsFrac              float64 // Fraction of region-allocated objects that fade.
	ScannedRegionAllocBytesFrac float64 // Fraction of region-allocated bytes that are scanned by the GC.
	RegionScanCostRatio         float64 // Ratio of the cost of scanning a region vs. the regular heap.
	FadeAllocsPointerDensity    float64 // Average pointer density of region-allocated objects that fade.

}

func deltaCPUFrac(prof AppProfile, scenario Scenario) float64 {
	return float64(prof.TotalCPU+deltaCPU(prof, scenario))/float64(prof.TotalCPU) - 1.0
}

func deltaCPU(prof AppProfile, scenario Scenario) time.Duration {
	var d time.Duration

	// Change in alloc costs.
	d += deltaAllocCPU(prof, scenario)

	// Reduced GC cost.
	d += time.Duration(float64(prof.GCCPU) * (1 - scenario.RegionAllocBytesFrac))
	// GC cost of scanning region memory.
	d += time.Duration(float64(prof.GCCPU) * scenario.RegionAllocBytesFrac * (scenario.FadeAllocBytesFrac + scenario.ScannedRegionAllocBytesFrac) * scenario.RegionScanCostRatio)
	// Subtract original full base GC cost.
	d -= prof.GCCPU

	// New write barrier (overestimate).
	d += wbTestCPU(prof.PointerWrites)

	// Fade cost.
	d += fadeCPU(
		uint64(float64(prof.Allocs)*scenario.RegionAllocsFrac*scenario.FadeAllocsFrac),
		uint64(scenario.FadeAllocsPointerDensity*float64(prof.AllocBytes)*scenario.RegionAllocBytesFrac*scenario.FadeAllocBytesFrac),
	)
	return d
}

func deltaAllocCPU(prof AppProfile, scenario Scenario) time.Duration {
	var d time.Duration

	// Bump alloc cost.
	d += bumpAllocCPU(
		uint64(scenario.RegionAllocsFrac*float64(prof.Allocs)),
		uint64(scenario.RegionAllocBytesFrac*float64(prof.AllocBytes)),
	)
	// Base alloc cost.
	d += baseAllocCPU(
		uint64((1-scenario.RegionAllocsFrac)*float64(prof.Allocs)),
		uint64((1-scenario.RegionAllocBytesFrac)*float64(prof.AllocBytes)),
	)
	// Subtract original full base alloc cost.
	d -= baseAllocCPU(prof.Allocs, prof.AllocBytes)
	return d
}

func bumpAllocCPU(o, b uint64) time.Duration {
	return time.Duration(8*float64(o) + 0.15*float64(b))
}

func baseAllocCPU(o, b uint64) time.Duration {
	return time.Duration(20*float64(o) + 0.08*float64(b))
}

func wbTestCPU(writes uint64) time.Duration {
	return time.Duration(4.5 * float64(writes))
}

func fadeCPU(o, p uint64) time.Duration {
	return time.Duration(40*float64(o) + 3.37*float64(p))
}

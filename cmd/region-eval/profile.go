// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "time"

type AppProfile struct {
	Name          string
	TotalCPU      time.Duration
	GCCPU         time.Duration
	AllocBytes    uint64
	Allocs        uint64
	PointerWrites uint64
}

var AppProfiles = []AppProfile{
	{
		Name:          "Tile38",
		TotalCPU:      time.Duration(1055.508 * 1e9),
		GCCPU:         time.Duration(106033 * 1e6),
		Allocs:        145783906,
		AllocBytes:    84299344536,
		PointerWrites: 3982888311,
	},
	{
		Name:          "etcd Put",
		TotalCPU:      time.Duration(4.683 * 4 * 1e9),
		GCCPU:         time.Duration(310.651 * 1e6),
		Allocs:        8838440,
		AllocBytes:    1027291400,
		PointerWrites: 38108457,
	},
	{
		Name:          "etcd STM",
		TotalCPU:      time.Duration(13.303 * 4 * 1e9),
		GCCPU:         time.Duration(4677.1 * 1e6),
		Allocs:        51522979,
		AllocBytes:    11645083144,
		PointerWrites: 446980825,
	},
	{
		Name:          "CockroachDB 300 kv0",
		TotalCPU:      time.Duration(87.029 * 8 * 1e9),
		GCCPU:         time.Duration(24808.369 * 1e6),
		Allocs:        428559454,
		AllocBytes:    55367775328,
		PointerWrites: 5961213414,
	},
	{
		Name:          "CockroachDB 300 kv50",
		TotalCPU:      time.Duration(97.484 * 8 * 1e9),
		GCCPU:         time.Duration(26663.114 * 1e6),
		Allocs:        967379582,
		AllocBytes:    70718196320,
		PointerWrites: 6446345731,
	},
	{
		Name:          "CockroachDB 300 kv95",
		TotalCPU:      time.Duration(82.688 * 8 * 1e9),
		GCCPU:         time.Duration(20728.359 * 1e6),
		Allocs:        368300343,
		AllocBytes:    40104479528,
		PointerWrites: 5636509516,
	},
	{
		Name:          "CockroachDB 100 kv0",
		TotalCPU:      time.Duration(103.433 * 8 * 1e9),
		GCCPU:         time.Duration(89548.573 * 1e6),
		Allocs:        1106189051,
		AllocBytes:    74973042400,
		PointerWrites: 5669424874,
	},
	{
		Name:          "CockroachDB 100 kv50",
		TotalCPU:      time.Duration(102.625 * 8 * 1e9),
		GCCPU:         time.Duration(80561.78 * 1e6),
		Allocs:        1052674597,
		AllocBytes:    69439903920,
		PointerWrites: 5709550463,
	},
	{
		Name:          "CockroachDB 100 kv95",
		TotalCPU:      time.Duration(123.101 * 8 * 1e9),
		GCCPU:         time.Duration(101636.461 * 1e6),
		Allocs:        1958662837,
		AllocBytes:    98514330368,
		PointerWrites: 6556261885,
	},
}

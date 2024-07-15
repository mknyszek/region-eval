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
		Name:          "Tile38Bench",
		TotalCPU:      time.Duration(1055.508 * 1e9),
		GCCPU:         time.Duration(106033 * 1e6),
		Allocs:        145783906,
		AllocBytes:    84299344536,
		PointerWrites: 3982888311,
	},
	{
		Name:          "EtcdPutBench",
		TotalCPU:      time.Duration(4.683 * 4 * 1e9),
		GCCPU:         time.Duration(310.651 * 1e6),
		Allocs:        8838440,
		AllocBytes:    1027291400,
		PointerWrites: 38108457,
	},
	{
		Name:          "EtcdSTMBench",
		TotalCPU:      time.Duration(13.303 * 4 * 1e9),
		GCCPU:         time.Duration(4677.1 * 1e6),
		Allocs:        51522979,
		AllocBytes:    11645083144,
		PointerWrites: 446980825,
	},
}

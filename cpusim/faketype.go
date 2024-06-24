// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpusim

import "github.com/mknyszek/region-eval/cpusim/bitmath"

type FakeType struct {
	Size_    uintptr
	PtrBytes uintptr
	_        [5]uintptr
	GCData   *byte
	_        [1]uintptr
}

var allFakeTypes []*FakeType

func NewFakeType(size, ptrs uintptr, gcdata []byte) *FakeType {
	if ptrs > size {
		panic("ptrs must be less than or equal to size")
	}
	if len(gcdata)%ptrSize != 0 {
		panic("gc data must be rounded to the pointer size")
	}
	gcd := (*byte)(nil)
	if len(gcdata) != 0 {
		gcd = &gcdata[0]
	}
	typ := &FakeType{
		Size_:    bitmath.AlignUp(size, ptrSize),
		PtrBytes: bitmath.AlignUp(ptrs, ptrSize),
		GCData:   gcd,
	}
	allFakeTypes = append(allFakeTypes, typ)
	return typ
}

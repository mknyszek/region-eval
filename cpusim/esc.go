// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpusim

import (
	"math/bits"
	"unsafe"

	"github.com/mknyszek/region-eval/cpusim/bitmath"
)

func MarkEscaped(a Pointer) {
	if a == nil {
		return
	}

	// Pull out the block metadata.
	base := bitmath.AlignDown(uintptr(a), BlockSize)
	objIdx := (uintptr(a) - base) / minAlign
	d := (*BlockMeta)(unsafe.Pointer(base))

	// Find the start of the object.
	//
	// Fast path: we're pointing to the start of the object (one word past the header).
	objStart := Pointer(nil)
	if objIdx != 0 && d.ObjBits[(objIdx-1)/64]&(1<<((objIdx-1)%64)) != 0 {
		objIdx -= 1
		objStart = Pointer(unsafe.Pointer(bitmath.AlignDown(uintptr(a), minAlign) - minAlign))
	} else {
		// We're not pointing to the start of the object.
		mask := (uint64(1) << objIdx) - 1
		n := uintptr(bits.LeadingZeros64(d.ObjBits[objIdx/64] & mask))

		// Iterate until we find the next bit.
		for n == 64 {
			objIdx = bitmath.AlignDown(objIdx, 64) - 64
			n = uintptr(bits.LeadingZeros64(d.ObjBits[objIdx/64]))
		}
		objIdx = bitmath.AlignDown(objIdx, 64) + 64 - n - 1
		objStart = Pointer(unsafe.Pointer(base + objIdx*minAlign))
	}
	header := *(*uint64)(objStart)
	size := uintptr(header>>48) * 8

	// Set the escaped bits.
	objEndIdx := objIdx + size/minAlign
	if objIdx/64 == objEndIdx/64 {
		// Fast path: small object that doesn't cross a bitmap word boundary.
		d.EscBits[objIdx/64] |= ((uint64(1) << (objEndIdx - objIdx + 1)) - 1) << (objIdx % 64)
	} else {
		_ = d.EscBits[objEndIdx/64]

		// Set leading bits.
		d.EscBits[objIdx/64] |= ^uint64(0) << (objIdx % 64)
		for k := objIdx/64 + 1; k < objEndIdx/64; k++ {
			d.EscBits[k] = ^uint64(0)
		}
		// Set trailing bits.
		d.EscBits[objEndIdx/64] |= (uint64(1) << (objEndIdx%64 + 1)) - 1
	}

	// Set the line escape bits.
	objLine := (uintptr(a) - base) / lineSize
	objEndLine := (uintptr(a) + size - base) / lineSize
	d.LineEscape |= uint64(1) << (objEndLine - objLine) << objLine

	// Nothing to transitively mark escaped.
	typ := (*FakeType)(unsafe.Pointer(uintptr(header & ((uint64(1) << 48) - 1))))
	if typ.PtrBytes == 0 {
		return
	}

	// Iterate over the object's pointers and transitively mark anything escaped.
	addr := uintptr(objStart) + headerSize
	limit := addr + size
	tp := typePointers{elem: addr, addr: addr, mask: readUintptr(typ.GCData), typ: typ}
	for {
		var addr uintptr
		if tp, addr = tp.nextFast(); addr == 0 {
			if tp, addr = tp.next(limit); addr == 0 {
				break
			}
		}
		ptr := *(*uintptr)(unsafe.Pointer(addr))
		if ptr != 0 {
			println("start", unsafe.Pointer(objStart), unsafe.Pointer(addr))
			println(unsafe.Pointer(ptr), unsafe.Pointer(typ))
			panic("wtf")
		}
		MarkEscaped(Pointer(ptr))
	}
}

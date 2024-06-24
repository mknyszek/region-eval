// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bitmath

func AlignUp(x, align uintptr) uintptr {
	return (x + align - 1) &^ (align - 1)
}

func AlignDown(x, align uintptr) uintptr {
	return x &^ (align - 1)
}

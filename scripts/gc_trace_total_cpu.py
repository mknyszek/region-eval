# Copyright 2024 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import sys

total_gc_cpu_ms = float(0)
for line in sys.stdin:
    if len(line.strip()) == 0:
        continue
    cpu = line.split(" ")[7]
    st, w, mt = cpu.split("+")
    a, d, i = w.split("/")
    total_gc_cpu_ms += float(st) + float(a) + float(d) + float(i) + float(mt)

print(total_gc_cpu_ms)

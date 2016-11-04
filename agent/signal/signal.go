// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package signal

const (
	// Stack represents a command to print stack trace.
	Stack = byte(0x1)

	// GC runs the garbage collector.
	GC = byte(0x2)

	// GCStats prints GC stats.
	GCStats = byte(0x3)
)

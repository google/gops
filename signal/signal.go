// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package signal contains signals used to communicate to the gops agents.
package signal

const (
	// StackTrace represents a command to print stack trace.
	StackTrace = byte((iota + 1))

	// GC runs the garbage collector.
	GC

	// MemStats reports memory stats.
	MemStats

	// Version prints the Go version.
	Version

	// HeapProfile starts `go tool pprof` with the current memory profile.
	HeapProfile

	// CPUProfile starts `go tool pprof` with the current CPU profile
	CPUProfile

	// Vitals returns Go runtime statistics such as number of goroutines, GOMAXPROCS, and NumCPU.
	Vitals
)

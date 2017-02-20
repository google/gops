// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Program gops is a tool to list currently running Go processes.
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/google/gops/internal"
	"github.com/google/gops/internal/objfile"
	ps "github.com/keybase/go-ps"
)

const helpText = `Usage: gops is a tool to list and diagnose Go processes.


Commands:
    stack       Prints the stack trace.
    gc          Runs the garbage collector and blocks until successful.
    memstats    Prints the allocation and garbage collection stats.
    version     Prints the Go version used to build the program.
    stats       Prints the vital runtime stats.
    help        Prints this help text.

Profiling commands:
    trace       Runs the runtime tracer for 5 secs and launches "go tool trace".
    pprof-heap  Reads the heap profile and launches "go tool pprof".
    pprof-cpu   Reads the CPU profile and launches "go tool pprof".


All commands require the agent running on the Go process.
Symbol "*" indicates the process runs the agent.`

// TODO(jbd): add link that explains the use of agent.

func main() {
	if len(os.Args) < 2 {
		processes()
		return
	}

	cmd := os.Args[1]
	if cmd == "help" {
		usage("")
	}
	if len(os.Args) < 3 {
		usage("missing PID or address")
	}
	fn, ok := cmds[cmd]
	if !ok {
		usage("unknown subcommand")
	}
	addr, err := targetToAddr(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't resolve addr or pid %v to TCPAddress: %v\n", os.Args[2], err)
		os.Exit(1)
	}
	if err := fn(*addr); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func processes() {
	pss, err := ps.Processes()
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(len(pss))

	for _, pr := range pss {
		pr := pr
		go func() {
			defer wg.Done()

			printIfGo(pr)
		}()
	}
	wg.Wait()
}

// printIfGo looks up the runtime.buildVersion symbol
// in the process' binary and determines if the process
// if a Go process or not. If the process is a Go process,
// it reports PID, binary name and full path of the binary.
func printIfGo(pr ps.Process) {
	if pr.Pid() == 0 {
		// ignore system process
		return
	}
	path, err := pr.Path()
	if err != nil {
		return
	}
	obj, err := objfile.Open(path)
	if err != nil {
		return
	}
	defer obj.Close()

	symbols, err := obj.Symbols()
	if err != nil {
		return
	}

	var ok bool
	for _, s := range symbols {
		if s.Name == "runtime.buildVersion" {
			ok = true
		}
	}

	var agent bool
	pidfile, err := internal.PIDFile(pr.Pid())
	if err == nil {
		_, err := os.Stat(pidfile)
		agent = err == nil
	}

	if ok {
		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf, "%d", pr.Pid())
		if agent {
			fmt.Fprint(buf, "*")
		}
		fmt.Fprintf(buf, "\t%v\t(%v)\n", pr.Executable(), path)
		buf.WriteTo(os.Stdout)
	}
}

func usage(msg string) {
	if msg != "" {
		fmt.Printf("gops: %v\n", msg)
	}
	fmt.Fprintf(os.Stderr, "%v\n", helpText)
	os.Exit(1)
}

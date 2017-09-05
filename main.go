// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Program gops is a tool to list currently running Go processes.
package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/gops/goprocess"
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
	ps := goprocess.FindAll()

	var maxPID, maxPPID, maxExec, maxVersion int
	for i, p := range ps {
		ps[i].BuildVersion = shortenVersion(p.BuildVersion)
		maxPID = max(maxPID, len(strconv.Itoa(p.PID)))
		maxPPID = max(maxPPID, len(strconv.Itoa(p.PPID)))
		maxExec = max(maxExec, len(p.Exec))
		maxVersion = max(maxVersion, len(ps[i].BuildVersion))

	}

	for _, p := range ps {
		buf := bytes.NewBuffer(nil)
		pid := strconv.Itoa(p.PID)
		fmt.Fprint(buf, pad(pid, maxPID))
		fmt.Fprint(buf, " ")
		ppid := strconv.Itoa(p.PPID)
		fmt.Fprint(buf, pad(ppid, maxPPID))
		fmt.Fprint(buf, " ")
		fmt.Fprint(buf, pad(p.Exec, maxExec))
		if p.Agent {
			fmt.Fprint(buf, "*")
		} else {
			fmt.Fprint(buf, " ")
		}
		fmt.Fprint(buf, " ")
		fmt.Fprint(buf, pad(p.BuildVersion, maxVersion))
		fmt.Fprint(buf, " ")
		fmt.Fprint(buf, p.Path)
		fmt.Fprintln(buf)
		buf.WriteTo(os.Stdout)
	}
}

var develRe = regexp.MustCompile(`devel\s+\+\w+`)

func shortenVersion(v string) string {
	if !strings.HasPrefix(v, "devel") {
		return v
	}
	results := develRe.FindAllString(v, 1)
	if len(results) == 0 {
		return v
	}
	return results[0]
}

func usage(msg string) {
	if msg != "" {
		fmt.Printf("gops: %v\n", msg)
	}
	fmt.Fprintf(os.Stderr, "%v\n", helpText)
	os.Exit(1)
}

func pad(s string, total int) string {
	if len(s) >= total {
		return s
	}
	return s + strings.Repeat(" ", total-len(s))
}

func max(i, j int) int {
	if i > j {
		return i
	}
	return j
}

// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/gops/goprocess"
)

const helpText = `gops is a tool to list and diagnose Go processes.

Usage:
  gops <cmd> <pid|addr> ...
  gops <pid> # displays process info
  gops help  # displays this help message

Commands:
  stack      Prints the stack trace.
  gc         Runs the garbage collector and blocks until successful.
  setgc	     Sets the garbage collection target percentage.
  memstats   Prints the allocation and garbage collection stats.
  version    Prints the Go version used to build the program.
  stats      Prints runtime stats.
  trace      Runs the runtime tracer for 5 secs and launches "go tool trace".
  pprof-heap Reads the heap profile and launches "go tool pprof".
  pprof-cpu  Reads the CPU profile and launches "go tool pprof".

All commands require the agent running on the Go process.
"*" indicates the process is running the agent.`

// TODO(jbd): add link that explains the use of agent.

// Execute the root command.
func Execute() {
	if len(os.Args) < 2 {
		processes()
		return
	}

	cmd := os.Args[1]

	// See if it is a PID.
	pid, err := strconv.Atoi(cmd)
	if err == nil {
		var period time.Duration
		if len(os.Args) >= 3 {
			period, err = time.ParseDuration(os.Args[2])
			if err != nil {
				secs, _ := strconv.Atoi(os.Args[2])
				period = time.Duration(secs) * time.Second
			}
		}
		processInfo(pid, period)
		return
	}

	if cmd == "help" {
		usage("")
	}

	if cmd == "tree" {
		displayProcessTree()
		return
	}

	fn, ok := cmds[cmd]
	if !ok {
		usage("unknown subcommand")
	}
	if len(os.Args) < 3 {
		usage("Missing PID or address.")
		os.Exit(1)
	}
	addr, err := targetToAddr(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't resolve addr or pid %v to TCPAddress: %v\n", os.Args[2], err)
		os.Exit(1)
	}
	var params []string
	if len(os.Args) > 3 {
		params = append(params, os.Args[3:]...)
	}
	if err := fn(*addr, params); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

var develRe = regexp.MustCompile(`devel\s+\+\w+`)

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
	// default exit code for the statement
	exitCode := 0
	if msg != "" {
		// founded an unexpected command
		fmt.Printf("gops: %v\n", msg)
		exitCode = 1
	}
	fmt.Fprintf(os.Stderr, "%v\n", helpText)
	os.Exit(exitCode)
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

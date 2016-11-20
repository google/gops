// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Program gops is a tool to list currently running Go processes.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/gops/internal/objfile"

	ps "github.com/keybase/go-ps"
)

const helpText = `Usage: gops is a tool to list and diagnose Go processes.

    gops                  Lists all Go processes currently running.
    gops [cmd] -p=<pid>   See the section below.

Commands:
    gc          Runs the garbage collector and blocks until successful.

    stack       Prints the stack trace.
    memstats    Prints the garbage collection stats.
    version     Prints the Go version used to build the program.
    vitals      Prints the runtime stats.

    pprof-heap  Reads the heap profile and launches "go tool pprof".
    pprof-cpu   Reads the CPU profile and launches "go tool pprof".

    help        Prints this help text.

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
	fn, ok := cmds[cmd]
	if !ok {
		usage("unknown subcommand")
	}

	var pid int
	flag.IntVar(&pid, "p", -1, "")
	flag.CommandLine.Parse(os.Args[2:])
	if pid == -1 {
		usage("missing -p=<pid> flag")
	}

	if err := fn(pid); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func processes() {
	pss, err := ps.Processes()
	if err != nil {
		log.Fatal(err)
	}
	var undetermined int
	for _, pr := range pss {
		if pr.Pid() == 0 {
			// ignore system process
			continue
		}
		path, err := pr.Path()
		if err != nil {
			undetermined++
			continue
		}
		ok, agent, err := isGo(pr.Pid(), path)
		if err != nil {
			// TODO(jbd): worth to report the number?
			continue
		}
		if ok {
			fmt.Printf("%d", pr.Pid())
			if agent {
				fmt.Printf("*")
			}
			fmt.Printf("\t%v\t(%v)\n", pr.Executable(), path)
		}
	}
	if undetermined > 0 {
		fmt.Printf("\n%d processes left undetermined\n", undetermined)
	}
}

func isGo(pid int, path string) (ok bool, agent bool, err error) {
	obj, err := objfile.Open(path)
	if err != nil {
		return false, false, err
	}
	defer obj.Close()

	symbols, err := obj.Symbols()
	if err != nil {
		return false, false, err
	}

	// TODO(jbd): find a faster way to determine Go programs.
	for _, s := range symbols {
		if s.Name == "runtime.buildVersion" {
			ok = true
		}
		if strings.HasPrefix(s.Name, "github.com/google/gops/agent") {
			agent = true
		}
	}
	return ok, agent, nil
}

func usage(msg string) {
	if msg != "" {
		fmt.Printf("gops: %v\n", msg)
	}
	fmt.Fprintf(os.Stderr, "%v\n", helpText)
	os.Exit(1)
}

// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Program gops is a tool to list currently running Go processes.
package main

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/gops/goprocess"
	"github.com/shirou/gopsutil/process"
	"github.com/xlab/treeprint"
)

const helpText = `gops is a tool to list and diagnose Go processes.

	gops <cmd> <pid|addr> ...
	gops <pid> # displays process info

Commands:
    stack       	Prints the stack trace.
    gc          	Runs the garbage collector and blocks until successful.
    setgc	        Sets the garbage collection target percentage.
    memstats    	Prints the allocation and garbage collection stats.
    version     	Prints the Go version used to build the program.
    stats       	Prints the vital runtime stats.
    help        	Prints this help text.

Profiling commands:
    trace       	Runs the runtime tracer for 5 secs and launches "go tool trace".
    pprof-heap  	Reads the heap profile and launches "go tool pprof".
    pprof-cpu   	Reads the CPU profile and launches "go tool pprof".


All commands require the agent running on the Go process.
Symbol "*" indicates the process runs the agent.`

// TODO(jbd): add link that explains the use of agent.

func main() {
	if len(os.Args) < 2 {
		processes()
		return
	}

	cmd := os.Args[1]

	// See if it is a PID.
	pid, err := strconv.Atoi(cmd)
	if err == nil {
		processInfo(pid)
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

func processInfo(pid int) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		log.Fatalf("Cannot read process info: %v", err)
	}
	if v, err := p.Parent(); err == nil {
		fmt.Printf("parent PID:\t%v\n", v.Pid)
	}
	if v, err := p.NumThreads(); err == nil {
		fmt.Printf("threads:\t%v\n", v)
	}
	if v, err := p.MemoryPercent(); err == nil {
		fmt.Printf("memory usage:\t%.3f%%\n", v)
	}
	if v, err := p.CPUPercent(); err == nil {
		fmt.Printf("cpu usage:\t%.3f%%\n", v)
	}
	if v, err := p.Username(); err == nil {
		fmt.Printf("username:\t%v\n", v)
	}
	if v, err := p.Cmdline(); err == nil {
		fmt.Printf("cmd+args:\t%v\n", v)
	}
	if v, err := elapsedTime(p); err == nil {
		fmt.Printf("elapsed time:\t%v\n", v)
	}
	if v, err := p.Connections(); err == nil {
		if len(v) > 0 {
			for _, conn := range v {
				fmt.Printf("local/remote:\t%v:%v <-> %v:%v (%v)\n",
					conn.Laddr.IP, conn.Laddr.Port, conn.Raddr.IP, conn.Raddr.Port, conn.Status)
			}
		}
	}
}

// elapsedTime shows the elapsed time of the process indicating how long the
// process has been running for.
func elapsedTime(p *process.Process) (string, error) {
	crtTime, err := p.CreateTime()
	if err != nil {
		return "", err
	}
	etime := time.Since(time.Unix(int64(crtTime/1000), 0))
	return fmtEtimeDuration(etime), nil
}

// pstree contains a mapping between the PPIDs and the child processes.
var pstree map[int][]goprocess.P

// displayProcessTree displays a tree of all the running Go processes.
func displayProcessTree() {
	ps := goprocess.FindAll()
	pstree = make(map[int][]goprocess.P)
	for _, p := range ps {
		pstree[p.PPID] = append(pstree[p.PPID], p)
	}
	tree := treeprint.New()
	tree.SetValue("...")
	seen := map[int]bool{}
	for _, p := range ps {
		constructProcessTree(p.PPID, p, seen, tree)
	}
	fmt.Println(tree.String())
}

// constructProcessTree constructs the process tree in a depth-first fashion.
func constructProcessTree(ppid int, process goprocess.P, seen map[int]bool, tree treeprint.Tree) {
	if seen[ppid] {
		return
	}
	seen[ppid] = true
	if ppid != process.PPID {
		output := strconv.Itoa(ppid) + " (" + process.Exec + ")" + " {" + process.BuildVersion + "}"
		if process.Agent {
			tree = tree.AddMetaBranch("*", output)
		} else {
			tree = tree.AddBranch(output)
		}
	} else {
		tree = tree.AddBranch(ppid)
	}
	for index := range pstree[ppid] {
		process := pstree[ppid][index]
		constructProcessTree(process.PID, process, seen, tree)
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

// fmtEtimeDuration formats etime's duration based on ps' format:
// [[DD-]hh:]mm:ss
// format specification: http://linuxcommand.org/lc3_man_pages/ps1.html
func fmtEtimeDuration(d time.Duration) string {
	days := d / (24 * time.Hour)
	hours := d % (24 * time.Hour)
	minutes := hours % time.Hour
	seconds := math.Mod(minutes.Seconds(), 60)
	var b bytes.Buffer
	if days > 0 {
		fmt.Fprintf(&b, "%02d-", days)
	}
	if hours/time.Hour > 0 {
		fmt.Fprintf(&b, "%02d:", hours/time.Hour)
	}
	fmt.Fprintf(&b, "%02d:", minutes/time.Minute)
	fmt.Fprintf(&b, "%02.0f", seconds)
	return b.String()
}

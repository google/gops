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

	"github.com/google/gops/goprocess"
	"github.com/spf13/cobra"
)

// NewRoot command.
func NewRoot() *cobra.Command {
	return &cobra.Command{
		Use:   "gops",
		Short: "gops is a tool to list and diagnose Go processes.",
		Example: `  gops <cmd> <pid|addr> ...
  gops <pid> # displays process info
  gops help  # displays this help message`,
		// TODO(jbd): add link that explains the use of agent.
		Run: func(cmd *cobra.Command, args []string) {
			processes()
		},
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

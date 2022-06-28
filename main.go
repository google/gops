// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Program gops is a tool to list currently running Go processes.
package main

import (
	"log"
	"os"
	"strconv"

	"github.com/google/gops/internal/cmd"
)

func main() {
	var root = cmd.NewRoot()
	root.AddCommand(cmd.ProcessCommand())
	root.AddCommand(cmd.TreeCommand())
	root.AddCommand(cmd.AgentCommands()...)

	// Legacy support for `gops <pid>` command.
	//
	// When the second argument is provided as int as opposed to a sub-command
	// (like proc, version, etc), gops command effectively shortcuts that
	// to `gops process <pid>`.
	if len(os.Args) > 1 {
		// See second argument appears to be a pid rather than a subcommand
		_, err := strconv.Atoi(os.Args[1])
		if err == nil {
			cmd.ProcessInfo(os.Args[1:]) // shift off the command name
			return
		}
	}

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}

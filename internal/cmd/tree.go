// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/google/gops/goprocess"
	"github.com/spf13/cobra"
	"github.com/xlab/treeprint"
)

// TreeCommand displays a process tree.
func TreeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tree",
		Short: "Display parent-child tree for Go processes.",
		Run: func(cmd *cobra.Command, args []string) {
			displayProcessTree()
		},
	}
}

// displayProcessTree displays a tree of all the running Go processes.
func displayProcessTree() {
	ps := goprocess.FindAll()
	sort.Slice(ps, func(i, j int) bool {
		return ps[i].PPID < ps[j].PPID
	})
	pstree := make(map[int][]goprocess.P, len(ps))
	for _, p := range ps {
		pstree[p.PPID] = append(pstree[p.PPID], p)
	}
	tree := treeprint.New()
	tree.SetValue("...")
	seen := map[int]bool{}
	for _, p := range ps {
		constructProcessTree(p.PPID, p, pstree, seen, tree)
	}
	fmt.Println(tree.String())
}

// constructProcessTree constructs the process tree in a depth-first fashion.
func constructProcessTree(ppid int, process goprocess.P, pstree map[int][]goprocess.P, seen map[int]bool, tree treeprint.Tree) {

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
		constructProcessTree(process.PID, process, pstree, seen, tree)
	}
}

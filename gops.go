// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Program gops is a tool to list currently running Go processes.
package main

import (
	"fmt"
	"log"

	"hello/gops/internal/objfile"

	ps "github.com/keybase/go-ps"
)

func main() {
	pss, err := ps.Processes()
	if err != nil {
		log.Fatal(err)
	}

	var undetermined int
	for _, pr := range pss {
		name, err := pr.Path()
		if err != nil {
			undetermined++
			continue
		}

		ok, err := isGo(name)
		if err != nil {
			continue
		}

		if ok {
			fmt.Printf("%d\t%v\n", pr.Pid(), pr.Executable())
		}
	}

	if undetermined > 0 {
		fmt.Printf("\n%d processes\n", undetermined)
	}
}

func isGo(filename string) (ok bool, err error) {
	obj, err := objfile.Open(filename)
	if err != nil {
		return false, err
	}
	defer obj.Close()

	symbols, err := obj.Symbols()
	if err != nil {
		return false, err
	}

	// TODO(jbd): find a faster way to determine Go programs
	// looping over the symbols is a joke.
	for _, s := range symbols {
		if s.Name == "runtime.buildVersion" {
			return true, nil
		}
	}
	return false, nil
}

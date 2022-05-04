// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package goprocess reports the Go processes running on a host.
package goprocess

import (
	"os"
	"sync"

	"github.com/google/gops/internal"
	ps "github.com/keybase/go-ps"
)

// P represents a Go process.
type P struct {
	PID          int
	PPID         int
	Exec         string
	Path         string
	BuildVersion string
	Agent        bool
}

// FindAll returns all the Go processes currently running on this host.
func FindAll() []P {
	const concurrencyLimit = 10 // max number of concurrent workers
	pss, err := ps.Processes()
	if err != nil {
		return nil
	}
	return findAll(pss, isGo, concurrencyLimit)
}

// Allows to inject isGo for testing.
type isGoFunc func(ps.Process) (path, version string, agent, ok bool, err error)

func findAll(pss []ps.Process, isGo isGoFunc, concurrencyLimit int) []P {
	input := make(chan ps.Process, len(pss))
	output := make(chan P, len(pss))

	for _, ps := range pss {
		input <- ps
	}
	close(input)

	var wg sync.WaitGroup
	wg.Add(concurrencyLimit) // used to wait for workers to be finished

	// Run concurrencyLimit of workers until there
	// is no more processes to be checked in the input channel.
	for i := 0; i < concurrencyLimit; i++ {
		go func() {
			defer wg.Done()

			for pr := range input {
				path, version, agent, ok, err := isGo(pr)
				if err != nil {
					// TODO(jbd): Return a list of errors.
					continue
				}
				if !ok {
					continue
				}
				output <- P{
					PID:          pr.Pid(),
					PPID:         pr.PPid(),
					Exec:         pr.Executable(),
					Path:         path,
					BuildVersion: version,
					Agent:        agent,
				}
			}
		}()
	}
	wg.Wait()     // wait until all workers are finished
	close(output) // no more results to be waited for

	var results []P
	for p := range output {
		results = append(results, p)
	}
	return results
}

// Find finds info about the process identified with the given PID.
func Find(pid int) (p P, ok bool, err error) {
	pr, err := ps.FindProcess(pid)
	if err != nil {
		return P{}, false, err
	}
	path, version, agent, ok, err := isGo(pr)
	if !ok {
		return P{}, false, nil
	}
	return P{
		PID:          pr.Pid(),
		PPID:         pr.PPid(),
		Exec:         pr.Executable(),
		Path:         path,
		BuildVersion: version,
		Agent:        agent,
	}, true, nil
}

// isGo looks up the runtime.buildVersion symbol
// in the process' binary and determines if the process
// if a Go process or not. If the process is a Go process,
// it reports PID, binary name and full path of the binary.
func isGo(pr ps.Process) (path, version string, agent, ok bool, err error) {
	if pr.Pid() == 0 {
		// ignore system process
		return
	}
	path, err = pr.Path()
	if err != nil {
		return
	}
	version, err = goVersion(path)
	if err != nil {
		return
	}
	ok = true
	pidfile, err := internal.PIDFile(pr.Pid())
	if err == nil {
		_, err := os.Stat(pidfile)
		agent = err == nil
	}
	return path, version, agent, ok, nil
}

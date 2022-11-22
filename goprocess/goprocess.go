// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package goprocess reports the Go processes running on a host.
package goprocess

import (
	"os"
	"sync"

	"github.com/google/gops/internal"
	"github.com/shirou/gopsutil/v3/process"
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
	pss, err := process.Processes()
	if err != nil {
		return nil
	}
	return findAll(pss, isGo, concurrencyLimit)
}

// Allows to inject isGo for testing.
type isGoFunc func(*process.Process) (path, version string, agent, ok bool, err error)

func findAll(pss []*process.Process, isGo isGoFunc, concurrencyLimit int) []P {
	input := make(chan *process.Process, len(pss))
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
				ppid, err := pr.Ppid()
				if err != nil {
					continue
				}
				name, err := pr.Name()
				if err != nil {
					continue
				}

				output <- P{
					PID:          int(pr.Pid),
					PPID:         int(ppid),
					Exec:         name,
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
	pr, err := process.NewProcess(int32(pid))
	if err != nil {
		return P{}, false, err
	}
	path, version, agent, ok, err := isGo(pr)
	if !ok || err != nil {
		return P{}, false, nil
	}
	ppid, err := pr.Ppid()
	if err != nil {
		return P{}, false, err
	}
	name, err := pr.Name()
	if err != nil {
		return P{}, false, err
	}
	return P{
		PID:          int(pr.Pid),
		PPID:         int(ppid),
		Exec:         name,
		Path:         path,
		BuildVersion: version,
		Agent:        agent,
	}, true, nil
}

// isGo looks up the runtime.buildVersion symbol
// in the process' binary and determines if the process
// if a Go process or not. If the process is a Go process,
// it reports PID, binary name and full path of the binary.
func isGo(pr *process.Process) (path, version string, agent, ok bool, err error) {
	if pr.Pid == 0 {
		// ignore system process
		return
	}
	path, err = pr.Exe()
	if err != nil {
		return
	}
	version, err = goVersion(path)
	if err != nil {
		return
	}
	ok = true
	pidfile, err := internal.PIDFile(int(pr.Pid))
	if err == nil {
		_, err := os.Stat(pidfile)
		agent = err == nil
	}
	return path, version, agent, ok, nil
}

// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package goprocess reports the Go processes running on a host.
package goprocess

import (
	"os"

	goversion "rsc.io/goversion/version"

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
	output := make(chan []P, 1)
	output <- nil
	// Using buffered channel as a semaphore to limit throughput.
	// See https://golang.org/doc/effective_go.html#channels
	type token struct{}
	sem := make(chan token, concurrencyLimit)
	for _, pr := range pss {
		sem <- token{}
		pr := pr
		go func() {
			defer func() { <-sem }()
			if path, version, agent, ok, err := isGo(pr); err != nil {
				// TODO(jbd): Return a list of errors.
			} else if ok {
				output <- append(<-output, P{
					PID:          pr.Pid(),
					PPID:         pr.PPid(),
					Exec:         pr.Executable(),
					Path:         path,
					BuildVersion: version,
					Agent:        agent,
				})
			}
		}()
	}
	// Acquire all semaphore slots to wait for work to complete.
	for n := cap(sem); n > 0; n-- {
		sem <- token{}
	}
	return <-output
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
	var versionInfo goversion.Version
	versionInfo, err = goversion.ReadExe(path)
	if err != nil {
		return
	}
	ok = true
	version = versionInfo.Release
	pidfile, err := internal.PIDFile(pr.Pid())
	if err == nil {
		_, err := os.Stat(pidfile)
		agent = err == nil
	}
	return path, version, agent, ok, nil
}

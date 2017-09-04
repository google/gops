// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package goprocess reports the Go processes running on a host.
package goprocess

import (
	"os"
	"sync"

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
	var results []P

	pss, err := ps.Processes()
	if err != nil {
		return results
	}

	var wg sync.WaitGroup
	wg.Add(len(pss))

	for _, pr := range pss {
		pr := pr
		go func() {
			defer wg.Done()

			path, version, agent, ok, err := isGo(pr)
			if err != nil {
				// TODO(jbd): Return a list of errors.
			}
			if !ok {
				return
			}
			results = append(results, P{
				PID:          pr.Pid(),
				PPID:         pr.PPid(),
				Exec:         pr.Executable(),
				Path:         path,
				BuildVersion: version,
				Agent:        agent,
			})
		}()
	}
	wg.Wait()
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

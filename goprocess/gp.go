// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package goprocess reports the Go processes running on a host.
package goprocess

import (
	"os"
	"sync"

	"github.com/google/gops/internal"
	"github.com/google/gops/internal/goversion"
	ps "github.com/keybase/go-ps"
)

// GoProcess represents a Go process.
type GoProcess struct {
	PID          int
	PPID         int
	Exec         string
	Path         string
	BuildVersion string
	Agent        bool
}

// Find returns all the Go processes currently
// running on this host.
func Find() []GoProcess {
	var results []GoProcess

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
			results = append(results, GoProcess{
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
	fi, err := os.Stat(path)
	if err != nil {
		return
	}
	version, ok = goversion.Report(path, path, fi)
	pidfile, err := internal.PIDFile(pr.Pid())
	if err == nil {
		_, err := os.Stat(pidfile)
		agent = err == nil
	}
	return path, version, agent, ok, nil
}

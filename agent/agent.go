// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package agent provides hooks programs can register to retrieve
// diagnostics data by using gops.
package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	gosignal "os/signal"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"sync"
	"time"

	"context"
	"github.com/google/gops/internal"
	"github.com/google/gops/signal"
	"github.com/kardianos/osext"
)

const defaultAddr = "127.0.0.1:0"

var (
	mu       sync.Mutex
	server   *http.Server
	listener net.Listener

	units = []string{" bytes", "KB", "MB", "GB", "TB", "PB"}
)

// Options allows configuring the started agent.
type Options struct {
	// Addr is the host:port the agent will be listening at.
	// Optional.
	Addr string

	// NoShutdownCleanup tells the agent not to automatically cleanup
	// resources if the running process receives an interrupt.
	// Optional.
	NoShutdownCleanup bool
}

// Listen starts the gops agent on a host process. Once agent started, users
// can use the advanced gops features. The agent will listen to Interrupt
// signals and exit the process, if you need to perform further work on the
// Interrupt signal use the options parameter to configure the agent
// accordingly.
//
// Note: The agent exposes an endpoint via a TCP connection that can be used by
// any program on the system. Review your security requirements before starting
// the agent.
func Listen(opts *Options) error {
	mu.Lock()
	defer mu.Unlock()

	if opts == nil {
		opts = &Options{}
	}

	gopsdir, err := internal.ConfigDir()
	if err != nil {
		return err
	}

	err = os.MkdirAll(gopsdir, os.ModePerm)
	if err != nil {
		return err
	}

	pid := os.Getpid()
	portfile, _ := internal.PIDFile(pid)
	if port, err := internal.GetPort(pid); err == nil {
		return fmt.Errorf("gops: agent already listening at port: %s", port)
	}

	addr := opts.Addr
	if addr == "" {
		addr = defaultAddr
	}

	srv := &http.Server{Addr: addr, Handler: &Agent{}}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	port := ln.Addr().(*net.TCPAddr).Port
	err = ioutil.WriteFile(portfile, []byte(strconv.Itoa(port)), os.ModePerm)
	if err != nil {
		return err
	}

	if !opts.NoShutdownCleanup {
		gracefulShutdown()
	}

	listener = ln
	server = srv
	go srv.Serve(tcpKeepAliveListener{ln.(*net.TCPListener)})

	return nil
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away. Copy from "net/http" package
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

func gracefulShutdown() {
	c := make(chan os.Signal, 1)
	gosignal.Notify(c, os.Interrupt)
	go func() {
		// cleanup the socket on shutdown.
		<-c
		Close()
		os.Exit(1)
	}()
}

func formatBytes(val uint64) string {
	var i int
	var target uint64
	for i = range units {
		target = 1 << uint(10*(i+1))
		if val < target {
			break
		}
	}
	if i > 0 {
		return fmt.Sprintf("%0.2f%s (%d bytes)", float64(val)/(float64(target)/1024), units[i], val)
	}
	return fmt.Sprintf("%d bytes", val)
}

// Agent implement http.Handler
type Agent struct{}

// ServeHTTP parse the requests body into signal.Command and run the task
func (a *Agent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cmd := signal.Command{}
	decodeErr := json.NewDecoder(r.Body).Decode(&cmd)
	if decodeErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	switch cmd.Code {
	case signal.StackTrace:
		pprof.Lookup("goroutine").WriteTo(w, 2)
	case signal.GC:
		runtime.GC()
		w.Write([]byte("ok"))
	case signal.MemStats:
		var s runtime.MemStats
		runtime.ReadMemStats(&s)
		fmt.Fprintf(w, "alloc: %v\n", formatBytes(s.Alloc))
		fmt.Fprintf(w, "total-alloc: %v\n", formatBytes(s.TotalAlloc))
		fmt.Fprintf(w, "sys: %v\n", formatBytes(s.Sys))
		fmt.Fprintf(w, "lookups: %v\n", s.Lookups)
		fmt.Fprintf(w, "mallocs: %v\n", s.Mallocs)
		fmt.Fprintf(w, "frees: %v\n", s.Frees)
		fmt.Fprintf(w, "heap-alloc: %v\n", formatBytes(s.HeapAlloc))
		fmt.Fprintf(w, "heap-sys: %v\n", formatBytes(s.HeapSys))
		fmt.Fprintf(w, "heap-idle: %v\n", formatBytes(s.HeapIdle))
		fmt.Fprintf(w, "heap-in-use: %v\n", formatBytes(s.HeapInuse))
		fmt.Fprintf(w, "heap-released: %v\n", formatBytes(s.HeapReleased))
		fmt.Fprintf(w, "heap-objects: %v\n", s.HeapObjects)
		fmt.Fprintf(w, "stack-in-use: %v\n", formatBytes(s.StackInuse))
		fmt.Fprintf(w, "stack-sys: %v\n", formatBytes(s.StackSys))
		fmt.Fprintf(w, "next-gc: when heap-alloc >= %v\n", formatBytes(s.NextGC))
		lastGC := "-"
		if s.LastGC != 0 {
			lastGC = fmt.Sprint(time.Unix(0, int64(s.LastGC)))
		}
		fmt.Fprintf(w, "last-gc: %v\n", lastGC)
		fmt.Fprintf(w, "gc-pause: %v\n", time.Duration(s.PauseTotalNs))
		fmt.Fprintf(w, "num-gc: %v\n", s.NumGC)
		fmt.Fprintf(w, "enable-gc: %v\n", s.EnableGC)
		fmt.Fprintf(w, "debug-gc: %v\n", s.DebugGC)
	case signal.Version:
		fmt.Fprintf(w, "%v\n", runtime.Version())
	case signal.HeapProfile:
		pprof.WriteHeapProfile(w)
	case signal.CPUProfile:
		if err := pprof.StartCPUProfile(w); err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		<-ctx.Done()
		pprof.StopCPUProfile()
	case signal.Stats:
		fmt.Fprintf(w, "goroutines: %v\n", runtime.NumGoroutine())
		fmt.Fprintf(w, "OS threads: %v\n", pprof.Lookup("threadcreate").Count())
		fmt.Fprintf(w, "GOMAXPROCS: %v\n", runtime.GOMAXPROCS(0))
		fmt.Fprintf(w, "num CPU: %v\n", runtime.NumCPU())
	case signal.BinaryDump:
		path, err := osext.Executable()
		if err != nil {
			return
		}
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()

		_, err = bufio.NewReader(f).WriteTo(w)
	case signal.Trace:
		trace.Start(w)
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		<-ctx.Done()
		trace.Stop()
	}
}

// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package agent provides hooks programs can register to retrieve
// diagnostics data by using gops.
package agent

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	gosignal "os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/google/gops/internal"
	"github.com/google/gops/signal"
)

var globalAgent agent

// AgentOptions allows configuring the started agent.
type AgentOptions struct {
	// HandleSignals is a boolean that tells whether the agent should listen to
	// the Interrupt signal and shutdown the applications after performing the
	// necessary cleanup.
	HandleSignals bool
}

// agent represents an agent that enable the advanced gops features
type agent struct {
	portfile string
	listener net.Listener
	options  AgentOptions
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
func Listen(options ...func(*AgentOptions)) error {
	opts := AgentOptions{
		HandleSignals: true,
	}
	for _, option := range options {
		option(&opts)
	}
	err := globalAgent.start()
	if err != nil {
		return err
	}
	return nil
}

func (a *agent) start() error {
	gopsdir, err := internal.ConfigDir()
	if err != nil {
		return err
	}
	err = os.MkdirAll(gopsdir, os.ModePerm)
	if err != nil {
		return err
	}

	a.listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := a.listener.Addr().(*net.TCPAddr).Port
	a.portfile = fmt.Sprintf("%s/%d", gopsdir, os.Getpid())
	err = ioutil.WriteFile(a.portfile, []byte(strconv.Itoa(port)), os.ModePerm)
	if err != nil {
		return err
	}

	if a.options.HandleSignals {
		c := make(chan os.Signal, 1)
		gosignal.Notify(c, os.Interrupt)
		go func() {
			// cleanup the socket on shutdown.
			<-c
			a.close()
			os.Exit(1)
		}()
	}

	listener := a.listener
	go func() {
		buf := make([]byte, 1)
		for {
			fd, err := listener.Accept()
			if err != nil {
				fmt.Fprintf(os.Stderr, "gops: %v", err)
				if netErr, ok := err.(net.Error); ok && !netErr.Temporary() {
					return
				}
				continue
			}
			if _, err := fd.Read(buf); err != nil {
				fmt.Fprintf(os.Stderr, "gops: %v", err)
				continue
			}
			if err := handle(fd, buf); err != nil {
				fmt.Fprintf(os.Stderr, "gops: %v", err)
				continue
			}
			fd.Close()
		}
	}()
	return err
}

// Close closes the agent, removing temporary files and closing the TCP
// listener.
func Close() {
	globalAgent.close()
}

func (a *agent) close() {
	if a.portfile != "" {
		os.Remove(a.portfile)
		a.portfile = ""
	}
	if a.listener != nil {
		a.listener.Close()
		a.listener = nil
	}
}

func handle(conn net.Conn, msg []byte) error {
	switch msg[0] {
	case signal.StackTrace:
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		_, err := conn.Write(buf[:n])
		return err
	case signal.GC:
		runtime.GC()
		_, err := conn.Write([]byte("ok"))
		return err
	case signal.MemStats:
		var s runtime.MemStats
		runtime.ReadMemStats(&s)
		fmt.Fprintf(conn, "alloc: %v bytes\n", s.Alloc)
		fmt.Fprintf(conn, "total-alloc: %v bytes\n", s.TotalAlloc)
		fmt.Fprintf(conn, "sys: %v bytes\n", s.Sys)
		fmt.Fprintf(conn, "lookups: %v\n", s.Lookups)
		fmt.Fprintf(conn, "mallocs: %v\n", s.Mallocs)
		fmt.Fprintf(conn, "frees: %v\n", s.Frees)
		fmt.Fprintf(conn, "heap-alloc: %v bytes\n", s.HeapAlloc)
		fmt.Fprintf(conn, "heap-sys: %v bytes\n", s.HeapSys)
		fmt.Fprintf(conn, "heap-idle: %v bytes\n", s.HeapIdle)
		fmt.Fprintf(conn, "heap-in-use: %v bytes\n", s.HeapInuse)
		fmt.Fprintf(conn, "heap-released: %v bytes\n", s.HeapReleased)
		fmt.Fprintf(conn, "heap-objects: %v\n", s.HeapObjects)
		fmt.Fprintf(conn, "stack-in-use: %v bytes\n", s.StackInuse)
		fmt.Fprintf(conn, "stack-sys: %v bytes\n", s.StackSys)
		fmt.Fprintf(conn, "next-gc: when heap-alloc >= %v bytes\n", s.NextGC)
		fmt.Fprintf(conn, "last-gc: %v ns\n", s.LastGC)
		fmt.Fprintf(conn, "gc-pause: %v ns\n", s.PauseTotalNs)
		fmt.Fprintf(conn, "num-gc: %v\n", s.NumGC)
		fmt.Fprintf(conn, "enable-gc: %v\n", s.EnableGC)
		fmt.Fprintf(conn, "debug-gc: %v\n", s.DebugGC)
	case signal.Version:
		fmt.Fprintf(conn, "%v\n", runtime.Version())
	case signal.HeapProfile:
		pprof.Lookup("heap").WriteTo(conn, 0)
	case signal.CPUProfile:
		if err := pprof.StartCPUProfile(conn); err != nil {
			return nil
		}
		time.Sleep(30 * time.Second)
		pprof.StopCPUProfile()
	case signal.Vitals:
		fmt.Fprintf(conn, "goroutines: %v\n", runtime.NumGoroutine())
		fmt.Fprintf(conn, "OS threads: %v\n", pprof.Lookup("threadcreate").Count())
		fmt.Fprintf(conn, "GOMAXPROCS: %v\n", runtime.GOMAXPROCS(0))
		fmt.Fprintf(conn, "num CPU: %v\n", runtime.NumCPU())
	}
	return nil
}

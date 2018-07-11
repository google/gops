// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/gops/internal"
	"github.com/google/gops/signal"
)

var cmds = map[string](func(addr net.TCPAddr, params []string) error){
	"stack":      stackTrace,
	"gc":         gc,
	"memstats":   memStats,
	"version":    version,
	"pprof-heap": pprofHeap,
	"pprof-cpu":  pprofCPU,
	"stats":      stats,
	"trace":      trace,
	"setgc":      setGC,
}

func setGC(addr net.TCPAddr, params []string) error {
	if len(params) != 1 {
		return errors.New("missing gc percentage")
	}
	perc, err := strconv.ParseInt(params[0], 10, strconv.IntSize)
	if err != nil {
		return err
	}
	buf := make([]byte, binary.MaxVarintLen64)
	binary.PutVarint(buf, perc)
	return cmdWithPrint(addr, signal.SetGCPercent, buf...)
}

func stackTrace(addr net.TCPAddr, _ []string) error {
	return cmdWithPrint(addr, signal.StackTrace)
}

func gc(addr net.TCPAddr, _ []string) error {
	_, err := cmd(addr, signal.GC)
	return err
}

func memStats(addr net.TCPAddr, _ []string) error {
	return cmdWithPrint(addr, signal.MemStats)
}

func version(addr net.TCPAddr, _ []string) error {
	return cmdWithPrint(addr, signal.Version)
}

func pprofHeap(addr net.TCPAddr, _ []string) error {
	return pprof(addr, signal.HeapProfile)
}

func pprofCPU(addr net.TCPAddr, _ []string) error {
	fmt.Println("Profiling CPU now, will take 30 secs...")
	return pprof(addr, signal.CPUProfile)
}

func trace(addr net.TCPAddr, _ []string) error {
	fmt.Println("Tracing now, will take 5 secs...")
	out, err := cmd(addr, signal.Trace)
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return errors.New("nothing has traced")
	}
	tmpfile, err := ioutil.TempFile("", "trace")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(tmpfile.Name(), out, 0); err != nil {
		return err
	}
	fmt.Printf("Trace dump saved to: %s\n", tmpfile.Name())
	// If go tool chain not found, stopping here and keep trace file.
	if _, err := exec.LookPath("go"); err != nil {
		return nil
	}
	defer os.Remove(tmpfile.Name())
	cmd := exec.Command("go", "tool", "trace", tmpfile.Name())
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func pprof(addr net.TCPAddr, p byte) error {

	tmpDumpFile, err := ioutil.TempFile("", "profile")
	if err != nil {
		return err
	}
	{
		out, err := cmd(addr, p)
		if err != nil {
			return err
		}
		if len(out) == 0 {
			return errors.New("failed to read the profile")
		}
		if err := ioutil.WriteFile(tmpDumpFile.Name(), out, 0); err != nil {
			return err
		}
		fmt.Printf("Profile dump saved to: %s\n", tmpDumpFile.Name())
		// If go tool chain not found, stopping here and keep dump file.
		if _, err := exec.LookPath("go"); err != nil {
			return nil
		}
		defer os.Remove(tmpDumpFile.Name())
	}
	// Download running binary
	tmpBinFile, err := ioutil.TempFile("", "binary")
	if err != nil {
		return err
	}
	{
		out, err := cmd(addr, signal.BinaryDump)
		if err != nil {
			return fmt.Errorf("failed to read the binary: %v", err)
		}
		if len(out) == 0 {
			return errors.New("failed to read the binary")
		}
		defer os.Remove(tmpBinFile.Name())
		if err := ioutil.WriteFile(tmpBinFile.Name(), out, 0); err != nil {
			return err
		}
	}
	fmt.Printf("Profiling dump saved to: %s\n", tmpDumpFile.Name())
	fmt.Printf("Binary file saved to: %s\n", tmpBinFile.Name())
	cmd := exec.Command("go", "tool", "pprof", tmpBinFile.Name(), tmpDumpFile.Name())
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func stats(addr net.TCPAddr, _ []string) error {
	return cmdWithPrint(addr, signal.Stats)
}

func cmdWithPrint(addr net.TCPAddr, c byte, params ...byte) error {
	out, err := cmd(addr, c, params...)
	if err != nil {
		return err
	}
	fmt.Printf("%s", out)
	return nil
}

// targetToAddr tries to parse the target string, be it remote host:port
// or local process's PID.
func targetToAddr(target string) (*net.TCPAddr, error) {
	if strings.Contains(target, ":") {
		// addr host:port passed
		var err error
		addr, err := net.ResolveTCPAddr("tcp", target)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse dst address: %v", err)
		}
		return addr, nil
	}
	// try to find port by pid then, connect to local
	pid, err := strconv.Atoi(target)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse PID: %v", err)
	}
	port, err := internal.GetPort(pid)
	if err != nil {
		return nil, fmt.Errorf("couldn't get port for PID %v: %v", pid, err)
	}
	addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:"+port)
	return addr, nil
}

func cmd(addr net.TCPAddr, c byte, params ...byte) ([]byte, error) {
	conn, err := cmdLazy(addr, c, params...)
	if err != nil {
		return nil, fmt.Errorf("couldn't get port by PID: %v", err)
	}

	all, err := ioutil.ReadAll(conn)
	if err != nil {
		return nil, err
	}
	return all, nil
}

func cmdLazy(addr net.TCPAddr, c byte, params ...byte) (io.Reader, error) {
	conn, err := net.DialTCP("tcp", nil, &addr)
	if err != nil {
		return nil, err
	}
	buf := []byte{c}
	buf = append(buf, params...)
	if _, err := conn.Write(buf); err != nil {
		return nil, err
	}
	return conn, nil
}

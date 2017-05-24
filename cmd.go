package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/gops/internal"
	"github.com/google/gops/signal"
)

var cmds = map[string](func(cli Client) error){
	"stack":      stackTrace,
	"gc":         gc,
	"memstats":   memStats,
	"version":    version,
	"pprof-heap": pprofHeap,
	"pprof-cpu":  pprofCPU,
	"stats":      stats,
	"trace":      trace,
}

func stackTrace(cli Client) error {
	return cmdWithPrint(cli, signal.StackTrace)
}

func gc(cli Client) error {
	_, err := cli.Run(signal.GC)
	return err
}

func memStats(cli Client) error {
	return cmdWithPrint(cli, signal.MemStats)
}

func version(cli Client) error {
	return cmdWithPrint(cli, signal.Version)
}

func pprofHeap(cli Client) error {
	return pprof(cli, signal.HeapProfile)
}

func pprofCPU(cli Client) error {
	fmt.Println("Profiling CPU now, will take 30 secs...")
	return pprof(cli, signal.CPUProfile)
}

func trace(cli Client) error {
	fmt.Println("Tracing now, will take 5 secs...")
	out, err := cli.Run(signal.Trace)
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
	defer os.Remove(tmpfile.Name())
	if err := ioutil.WriteFile(tmpfile.Name(), out, 0); err != nil {
		return err
	}
	cmd := exec.Command("go", "tool", "trace", tmpfile.Name())
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func pprof(cli Client, p byte) error {

	tmpDumpFile, err := ioutil.TempFile("", "profile")
	if err != nil {
		return err
	}
	{
		out, err := cli.Run(p)
		if err != nil {
			return err
		}
		if len(out) == 0 {
			return errors.New("failed to read the profile")
		}
		defer os.Remove(tmpDumpFile.Name())
		if err := ioutil.WriteFile(tmpDumpFile.Name(), out, 0); err != nil {
			return err
		}
	}
	// Download running binary
	tmpBinFile, err := ioutil.TempFile("", "binary")
	if err != nil {
		return err
	}
	{
		out, err := cli.Run(signal.BinaryDump)
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
	cmd := exec.Command("go", "tool", "pprof", tmpBinFile.Name(), tmpDumpFile.Name())
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func stats(cli Client) error {
	return cmdWithPrint(cli, signal.Stats)
}

func cmdWithPrint(cli Client, c byte) error {
	out, err := cli.Run(c)
	if err != nil {
		return err
	}
	fmt.Printf("%s", out)
	return nil
}

// targetToClient tries to parse the target string, be it remote host:port
// or local process's PID.
func targetToClient(target string) (Client, error) {
	if strings.HasPrefix(target, "http:") || strings.HasPrefix(target, "https:") {
		return &ClientHTTP{baseAddr: target}, nil
	}
	if strings.Index(target, ":") != -1 {
		// addr host:port passed
		var err error
		addr, err := net.ResolveTCPAddr("tcp", target)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse dst address: %v", err)
		}
		return &ClientTCP{addr: *addr}, nil
	}
	// try to find port by pid then, connect to local
	pid, err := strconv.Atoi(target)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse PID: %v", err)
	}
	port, err := internal.GetPort(pid)
	if err != nil {
		return nil, err
	}
	addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:"+port)
	return &ClientTCP{addr: *addr}, nil
}

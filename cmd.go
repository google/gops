package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/gops/internal"
	"github.com/google/gops/signal"
	"github.com/pkg/errors"
)

var cmds = map[string](func(target string) error){
	"stack":      stackTrace,
	"gc":         gc,
	"memstats":   memStats,
	"version":    version,
	"pprof-heap": pprofHeap,
	"pprof-cpu":  pprofCPU,
	"stats":      stats,
}

func stackTrace(target string) error {
	return cmdWithPrint(target, signal.StackTrace)
}

func gc(target string) error {
	addr, err := targetToAddr(target)
	if err != nil {
		return errors.Wrap(err, "Couldn't parse target's string to addr")
	}
	_, err = cmd(*addr, signal.GC)
	return err
}

func memStats(target string) error {
	return cmdWithPrint(target, signal.MemStats)
}

func version(target string) error {
	return cmdWithPrint(target, signal.Version)
}

func pprofHeap(target string) error {
	return pprof(target, signal.HeapProfile)
}

func pprofCPU(target string) error {
	fmt.Println("Profiling CPU now, will take 30 secs...")
	return pprof(target, signal.CPUProfile)
}

func pprof(target string, p byte) error {
	addr, err := targetToAddr(target)
	if err != nil {
		return errors.Wrap(err, "Couldn't parse target's string to addr")
	}

	tmpDumpFile, err := ioutil.TempFile("", "profile")
	if err != nil {
		return err
	}
	{
		out, err := cmd(*addr, p)
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

		out, err := cmd(*addr, signal.BinaryDump)
		if err != nil {
			return errors.New("Couldn't retrieve running binary's dump")
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

func stats(target string) error {
	return cmdWithPrint(target, signal.Stats)
}

func cmdWithPrint(target string, c byte) error {
	addr, err := targetToAddr(target)
	if err != nil {
		return errors.Wrap(err, "Couldn't parse target's string to addr")
	}
	out, err := cmd(*addr, c)
	if err != nil {
		return err
	}
	fmt.Printf("%s", out)
	return nil
}

// targetToAddr tries to parse the target string, be it remote host:port
// or local process's PID.
func targetToAddr(target string) (*net.TCPAddr, error) {
	if strings.Index(target, ":") != -1 {
		// addr host:port passed
		var err error
		addr, err := net.ResolveTCPAddr("tcp", target)
		if err != nil {
			return nil, errors.Wrap(err, "Couldn't parse dst address")
		}
		return addr, nil
	}
	// try to find port by pid then, connect to local
	pid, err := strconv.Atoi(target)
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't parse PID")
	}
	port, err := internal.GetPort(pid)
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't get port by PID")
	}
	addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:"+port)
	return addr, nil
}

func cmd(addr net.TCPAddr, c byte) ([]byte, error) {
	conn, err := net.DialTCP("tcp", nil, &addr)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte{c}); err != nil {
		return nil, err
	}
	all, err := ioutil.ReadAll(conn)
	if err != nil {
		return nil, err
	}
	return all, nil
}

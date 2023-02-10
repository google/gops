// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/gops/internal"
	"github.com/google/gops/signal"
	"github.com/spf13/cobra"
)

// AgentCommands is a bridge between the legacy multiplexing to commands, and
// full migration to Cobra for each command.
//
// The code is already nicely structured with one function per command so it
// seemed cleaner to combine them all together here and "generate" cobra
// commands as just thin wrappers, rather through individual constructors.
func AgentCommands() []*cobra.Command {
	var res []*cobra.Command

	var cmds = []legacyCommand{
		{
			name:  "stack",
			short: "Prints the stack trace.",
			fn:    stackTrace,
		},
		{
			name:  "gc",
			short: "Runs the garbage collector and blocks until successful.",
			fn:    gc,
		},
		{
			name:  "setgc",
			short: "Sets the garbage collection target percentage. To completely stop GC, set to 'off'",
			fn:    setGC,
		},
		{
			name:  "memstats",
			short: "Prints the allocation and garbage collection stats.",
			fn:    memStats,
		},
		{
			name:  "stats",
			short: "Prints runtime stats.",
			fn:    stats,
		},
		{
			name:  "trace",
			short: "Runs the runtime tracer for 5 secs and launches \"go tool trace\".",
			fn:    trace,
		},
		{
			name:  "pprof-heap",
			short: "Reads the heap profile and launches \"go tool pprof\".",
			fn:    pprofHeap,
		},
		{
			name:  "pprof-cpu",
			short: "Reads the CPU profile and launches \"go tool pprof\".",
			fn:    pprofCPU,
		},
		{
			name:  "version",
			short: "Prints the Go version used to build the program.",
			fn:    version,
		},
	}

	for _, c := range cmds {
		c := c
		res = append(res, &cobra.Command{
			Use:   fmt.Sprintf("%s <pid|addr>", c.name),
			Short: c.short,

			RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) < 1 {
					return fmt.Errorf("missing PID or address")
				}

				addr, err := targetToAddr(args[0])
				if err != nil {
					return fmt.Errorf(
						"couldn't resolve addr or pid %v to TCPAddress: %v\n", args[0], err,
					)
				}

				var params []string
				if len(args) > 1 {
					params = append(params, args[1:]...)
				}

				if err := c.fn(*addr, params); err != nil {
					return err
				}

				return nil
			},

			// errors get double printed otherwise
			SilenceUsage:  true,
			SilenceErrors: true,
		})
	}

	return res
}

type legacyCommand struct {
	name  string
	short string
	fn    func(addr net.TCPAddr, params []string) error
}

func setGC(addr net.TCPAddr, params []string) error {
	if len(params) != 1 {
		return errors.New("missing gc percentage")
	}
	var (
		perc int64
		err  error
	)
	if strings.ToLower(params[0]) == "off" {
		perc = -1
	} else {
		perc, err = strconv.ParseInt(params[0], 10, strconv.IntSize)
		if err != nil {
			return err
		}
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
	return pprof(addr, signal.HeapProfile, "heap")
}

func pprofCPU(addr net.TCPAddr, _ []string) error {
	fmt.Println("Profiling CPU now, will take 30 secs...")
	return pprof(addr, signal.CPUProfile, "cpu")
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
	tmpfile, err := os.CreateTemp("", "trace")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpfile.Name(), out, 0); err != nil {
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

func pprof(addr net.TCPAddr, p byte, prefix string) error {
	tmpDumpFile, err := os.CreateTemp("", prefix+"_profile")
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
		if err := os.WriteFile(tmpDumpFile.Name(), out, 0); err != nil {
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
	tmpBinFile, err := os.CreateTemp("", "binary")
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
		if err := os.WriteFile(tmpBinFile.Name(), out, 0); err != nil {
			return err
		}
	}
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

	all, err := io.ReadAll(conn)
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

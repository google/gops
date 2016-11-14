package main

import (
	"fmt"
	"io/ioutil"
	"net"

	"os/exec"

	"os"

	"github.com/google/gops/signal"
)

var cmds = map[string](func(pid int) error){
	"stack":    stackTrace,
	"gc":       gc,
	"memstats": memStats,
	"version":  version,
	"pprof":    pprof,
}

func stackTrace(pid int) error {
	out, err := cmd(pid, signal.StackTrace)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func gc(pid int) error {
	_, err := cmd(pid, signal.GC)
	return err
}

func memStats(pid int) error {
	out, err := cmd(pid, signal.MemStats)
	if err != nil {
		return err
	}
	fmt.Printf(out)
	return nil
}

func version(pid int) error {
	out, err := cmd(pid, signal.Version)
	if err != nil {
		return err
	}
	fmt.Printf(out)
	return nil
}

func pprof(pid int) error {
	out, err := cmd(pid, signal.HeapProfile)
	if err != nil {
		return err
	}
	tmpfile, err := ioutil.TempFile("", "heap-profile")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(tmpfile.Name(), []byte(out), 0); err != nil {
		return err
	}
	cmd := exec.Command("go", "tool", "pprof", tmpfile.Name())
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cmd(pid int, c byte) (string, error) {
	sock := fmt.Sprintf("/tmp/gops%d.sock", pid)
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return "", err
	}
	if _, err := conn.Write([]byte{c}); err != nil {
		return "", err
	}
	all, err := ioutil.ReadAll(conn)
	if err != nil {
		return "", err
	}
	return string(all), nil
}

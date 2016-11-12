package main

import (
	"fmt"
	"io/ioutil"
	"net"

	"github.com/google/gops/signal"
)

var cmds = map[string](func(pid int) error){
	"stack":    stackTrace,
	"gc":       gc,
	"memstats": memStats,
	"version":  version,
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

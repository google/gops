// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package agent provides hooks programs can register to retrieve
// diagnostics data by using gops.
package agent

import (
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
)

const (
	// Stack represents a command to print stack trace.
	Stack = byte(0x1)
)

func init() {
	sock := fmt.Sprintf("/tmp/gops%d.sock", os.Getpid())
	l, err := net.Listen("unix", sock)
	if err != nil {
		log.Fatal(err)
	}
	// TODO(jbd): cleanup the socket on shutdown.
	go func() {
		buf := make([]byte, 1)
		for {
			fd, err := l.Accept()
			if err != nil {
				log.Println(err)
				continue
			}
			if _, err := fd.Read(buf); err != nil {
				log.Println(err)
				continue
			}
			if err := handle(fd, buf); err != nil {
				log.Println(err)
				continue
			}
			fd.Close()
		}
	}()
}

func handle(conn net.Conn, msg []byte) error {
	switch msg[0] {
	case Stack:
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		_, err := conn.Write(buf[:n])
		return err
	}
	return nil
}

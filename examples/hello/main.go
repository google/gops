// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"time"

	"net/http"

	"net"

	"math"

	"io/ioutil"

	"github.com/google/gops/agent"
)

func main() {

	l, err := net.Listen("tcp", "127.0.0.1:12345")
	if err != nil {
		log.Fatal(err)
	}
	go doWork()
	go http.Serve(l, agent.HandlerFunc())

	if err := agent.Listen(nil); err != nil {
		log.Fatal(err)
	}
	time.Sleep(time.Hour)
}

func doWork() {
	// Emulate some work for non-empty profile
	for i := 0; ; i++ {
		res := math.Log(float64(i))
		ioutil.Discard.Write([]byte(fmt.Sprint(res)))
		<-time.After(time.Millisecond * 50)
	}
}

// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package agent

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestListen(t *testing.T) {
	err := Listen()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAgentListen(t *testing.T) {
	err := Listen(func(opts *AgentOptions) {
		opts.HandleSignals = false
	})
	if err != nil {
		t.Fatal(err)
	}
	if globalAgent.options.HandleSignals {
		t.Fatal("expected HandleSignals to be false")
	}
	portfile := globalAgent.portfile
	listener := globalAgent.listener
	portdata, err := ioutil.ReadFile(portfile)
	if err != nil {
		t.Fatal(err)
	}
	if len(portdata) == 0 || !strings.HasSuffix(listener.Addr().String(), string(portdata)) {
		t.Fatalf("expected portdata to have listened port, got: %q", string(portdata))
	}
}

func TestAgentClose(t *testing.T) {
	err := Listen(func(opts *AgentOptions) {
		opts.HandleSignals = false
	})
	if err != nil {
		t.Fatal(err)
	}
	portfile := globalAgent.portfile
	listener := globalAgent.listener
	Close()
	_, err = os.Stat(portfile)
	if !os.IsNotExist(err) {
		t.Fatalf("expected portfile not to exist, got error: %v", err)
	}
	if globalAgent.portfile != "" {
		t.Fatalf("expected portfile in agent to be empty, got: %q", globalAgent.portfile)
	}
	err = listener.Close()
	if err == nil || !strings.HasSuffix(err.Error(), "use of closed network connection") {
		t.Fatalf("expected listener not to closed, got error: %v", err)
	}
	if globalAgent.listener != nil {
		t.Fatalf("expected listener in agent to be nil, got: %#v", globalAgent.listener)
	}
}

func TestAgentListenMultipleClose(t *testing.T) {
	err := Listen(func(opts *AgentOptions) {
		opts.HandleSignals = false
	})
	if err != nil {
		t.Fatal(err)
	}
	Close()
	Close()
	Close()
	Close()
}

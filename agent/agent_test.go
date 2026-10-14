// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package agent

import (
	"os"
	"testing"
)

func TestListen(t *testing.T) {
	err := Listen(Options{})
	if err != nil {
		t.Fatal(err)
	}
	Close()
}

func TestAgentClose(t *testing.T) {
	err := Listen(Options{})
	if err != nil {
		t.Fatal(err)
	}
	Close()
	_, err = os.Stat(portfile)
	if !os.IsNotExist(err) {
		t.Fatalf("portfile = %q doesn't exist; err = %v", portfile, err)
	}
	if portfile != "" {
		t.Fatalf("got = %q; want empty portfile", portfile)
	}
}

func TestUseCustomConfigDir(t *testing.T) {
	err := Listen(Options{
		ConfigDir:       os.TempDir(),
		ShutdownCleanup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	Close()
}

func TestAgentListenMultipleClose(t *testing.T) {
	err := Listen(Options{})
	if err != nil {
		t.Fatal(err)
	}
	Close()
	Close()
	Close()
	Close()
}

func TestAgentListenReuseAddrAndPort(t *testing.T) {
	err := Listen(Options{
		Addr:                   "127.0.0.1:50000",
		ReuseSocketAddrAndPort: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	Close()
}

func TestAgentAddrAndPort(t *testing.T) {
	if Addr() != nil {
		t.Fatalf("Addr() = %v; want nil before Listen", Addr())
	}
	if _, err := Port(); err == nil {
		t.Fatal("Port() expected error before Listen, got nil")
	}

	err := Listen(Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}

	addr := Addr()
	if addr == nil {
		t.Fatal("Addr() = nil; want non-nil after Listen")
	}

	port, err := Port()
	if err != nil {
		t.Fatalf("Port() unexpected error: %v", err)
	}
	if port <= 0 {
		t.Fatalf("Port() = %d; want > 0", port)
	}

	Close()

	if Addr() != nil {
		t.Fatalf("Addr() = %v; want nil after Close", Addr())
	}
	if _, err := Port(); err == nil {
		t.Fatal("Port() expected error after Close, got nil")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		val  uint64
		want string
	}{
		{1023, "1023 bytes"},
		{1024, "1.00KB (1024 bytes)"},
		{1024*1024 - 100, "1023.90KB (1048476 bytes)"},
		{1024 * 1024, "1.00MB (1048576 bytes)"},
		{1024 * 1025, "1.00MB (1049600 bytes)"},
		{1024 * 1024 * 1024, "1.00GB (1073741824 bytes)"},
		{1024*1024*1024 + 430*1024*1024, "1.42GB (1524629504 bytes)"},
		{1024 * 1024 * 1024 * 1024 * 1024, "1.00PB (1125899906842624 bytes)"},
		{1024 * 1024 * 1024 * 1024 * 1024 * 1024, "1024.00PB (1152921504606846976 bytes)"},
	}
	for _, tt := range tests {
		result := formatBytes(tt.val)
		if result != tt.want {
			t.Errorf("formatBytes(%v) = %q; want %q", tt.val, result, tt.want)
		}
	}
}

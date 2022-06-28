// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandPresence(t *testing.T) {
	cmd := &cobra.Command{Use: "gops"}
	cmd.AddCommand(AgentCommands()...)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Error(err)
	}

	// basic check to make sure all the legacy commands have been ported over
	// it doesn't test they are correctly _implemented_, just that they are not
	// missing.
	wants := []string{
		"completion", "gc", "memstats", "pprof-cpu", "pprof-heap", "setgc",
		"stack", "stats", "trace", "version",
	}
	outs := out.String()
	for _, want := range wants {
		if !strings.Contains(outs, want) {
			t.Errorf("%q command not found in help", want)
		}
	}
}

// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Program gops is a tool to list currently running Go processes.
package main

import (
	"github.com/google/gops/internal/cmd"
)

func main() {
	cmd.Execute()
}

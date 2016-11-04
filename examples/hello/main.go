// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"time"

	_ "hello/gops/agent"
)

func main() {
	time.Sleep(time.Hour)
}

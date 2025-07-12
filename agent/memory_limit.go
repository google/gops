// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.19
// +build go1.19

package agent

import "runtime/debug"

func setMemoryLimit(limit int64) (int64, error) {
	return debug.SetMemoryLimit(limit), nil
}

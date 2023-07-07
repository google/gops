// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !go1.19
// +build !go1.19

package agent

import (
	"errors"
	"math"
)

func setMemoryLimit(limit int64) (int64, error) {
	if limit < 0 {
		return math.MaxInt64, nil
	}
	return 0, errors.New("memory limit not supported on Go versions before 1.19")
}

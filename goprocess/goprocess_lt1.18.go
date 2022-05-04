// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !go1.18
// +build !go1.18

package goprocess

import goversion "rsc.io/goversion/version"

func goVersion(path string) (string, error) {
	versionInfo, err := goversion.ReadExe(path)
	if err != nil {
		return "", err
	}
	return versionInfo.Release, nil
}

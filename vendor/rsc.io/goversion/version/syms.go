// Copyright 2017 The Go Authors. All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import "fmt"

func Syms(file string) ([]string, error) {
	f, err := openExe(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	syms, err := f.Symbols()
	if err != nil {
		return nil, err
	}

	for _, sym := range syms {
		fmt.Println(sym)
	}
	return nil, nil
}

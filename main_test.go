// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func Test_shortenVersion(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{
			version: "go1.8.1.typealias",
			want:    "go1.8.1.typealias",
		},
		{
			version: "go1.9",
			want:    "go1.9",
		},
		{
			version: "go1.9rc",
			want:    "go1.9rc",
		},
		{
			version: "devel +990dac2723 Fri Jun 30 18:24:58 2017 +0000",
			want:    "devel +990dac2723",
		},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := shortenVersion(tt.version); got != tt.want {
				t.Errorf("shortenVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

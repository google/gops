// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"testing"
	"time"
)

func Test_fmtEtimeDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{
			want: "00:00",
		},

		{
			d:    2*time.Minute + 5*time.Second + 400*time.Millisecond,
			want: "02:05",
		},
		{
			d:    1*time.Second + 500*time.Millisecond,
			want: "00:02",
		},
		{
			d:    2*time.Hour + 42*time.Minute + 12*time.Second,
			want: "02:42:12",
		},
		{
			d:    24 * time.Hour,
			want: "01-00:00:00",
		},
		{
			d:    24*time.Hour + 59*time.Minute + 59*time.Second,
			want: "01-00:59:59",
		},
	}
	for _, tt := range tests {
		t.Run(tt.d.String(), func(t *testing.T) {
			if got := fmtEtimeDuration(tt.d); got != tt.want {
				t.Errorf("fmtEtimeDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

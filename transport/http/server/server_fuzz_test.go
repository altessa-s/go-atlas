// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"testing"
	"time"
)

func FuzzWithReadTimeout(f *testing.F) {
	f.Add(int64(time.Second))
	f.Add(int64(0))
	f.Add(int64(-1))

	f.Fuzz(func(t *testing.T, ns int64) {
		opts := newOptions(WithReadTimeout(time.Duration(ns)))
		if opts == nil {
			t.Fatal("newOptions returned nil")
		}
	})
}

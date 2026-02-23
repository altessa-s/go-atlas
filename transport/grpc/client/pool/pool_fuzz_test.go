// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"testing"
	"time"
)

func FuzzWithSize(f *testing.F) {
	f.Add(1)
	f.Add(0)
	f.Add(-1)
	f.Add(100)

	f.Fuzz(func(t *testing.T, size int) {
		p := New(WithSize(size))
		if p == nil {
			t.Fatal("New() returned nil")
		}
	})
}

func FuzzWithMaxIdleTime(f *testing.F) {
	f.Add(int64(time.Second))
	f.Add(int64(0))
	f.Add(int64(-1))

	f.Fuzz(func(t *testing.T, ns int64) {
		p := New(WithMaxIdleTime(time.Duration(ns)))
		if p == nil {
			t.Fatal("New() returned nil")
		}
	})
}

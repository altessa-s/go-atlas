// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spool_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/core/io/spool"
)

func BenchmarkNew_InMemory(b *testing.B) {
	payload := []byte(strings.Repeat("x", 1024))
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()

	for b.Loop() {
		sp, err := spool.New(bytes.NewReader(payload))
		if err != nil {
			b.Fatal(err)
		}
		sp.Close()
	}
}

// BenchmarkNew_Spill measures the spill path where content exceeds the memory
// threshold and is written to a temp file.
func BenchmarkNew_Spill(b *testing.B) {
	payload := []byte(strings.Repeat("x", 8<<20)) // 8 MiB
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()

	for b.Loop() {
		sp, err := spool.New(bytes.NewReader(payload), spool.WithMemThreshold(4<<20))
		if err != nil {
			b.Fatal(err)
		}
		sp.Close()
	}
}

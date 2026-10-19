// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/retryqueue/storages/memory"
	"github.com/altessa-s/go-atlas/data/retryqueue/storages/storagetest"
)

// BenchmarkListDue measures a sweep's due-list read over 10k items, half due.
func BenchmarkListDue(b *testing.B) {
	s := memory.New[storagetest.Payload]()
	for i := range 10_000 {
		next := storagetest.T0.Add(time.Duration(i%2) * time.Hour)
		if _, err := s.Mark(b.Context(), storagetest.Pending("k"+strconv.Itoa(i), next)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.ListDue(b.Context(), storagetest.T0, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReschedule measures one fenced transition.
func BenchmarkReschedule(b *testing.B) {
	s := memory.New[storagetest.Payload]()
	if _, err := s.Mark(b.Context(), storagetest.Pending("k", storagetest.T0)); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := s.Reschedule(b.Context(), "k", 1, storagetest.T0); err != nil {
			b.Fatal(err)
		}
	}
}

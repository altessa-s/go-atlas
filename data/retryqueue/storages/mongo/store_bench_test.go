// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/data/retryqueue/storages/storagetest"
)

// BenchmarkListDue measures a sweep's due-list read of 200 items over the
// partial index; it skips when MONGO_URI is unset.
func BenchmarkListDue(b *testing.B) {
	s := newStore[storagetest.Payload](b, newColl(b))
	for i := range 400 {
		next := t0
		if i%2 == 1 {
			next = t0.Add(1 << 40)
		}
		if _, err := s.Mark(b.Context(), storagetest.Pending("k"+strconv.Itoa(i), next)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.ListDue(b.Context(), t0, 0); err != nil {
			b.Fatal(err)
		}
	}
}

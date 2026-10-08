// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"strconv"
	"testing"

	mongostorage "github.com/altessa-s/go-atlas/data/idempotency/storages/mongo"
)

func BenchmarkAttemptLockComplete(b *testing.B) {
	s, err := mongostorage.New(testDB(b))
	if err != nil {
		b.Fatal(err)
	}
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		i++
		key := "bench-" + strconv.Itoa(i)
		val := []byte("lock-" + key)
		if _, _, _, err := s.AttemptLock(b.Context(), key, val); err != nil {
			b.Fatal(err)
		}
		if err := s.Complete(b.Context(), key, []byte("done"), val); err != nil {
			b.Fatal(err)
		}
	}
}

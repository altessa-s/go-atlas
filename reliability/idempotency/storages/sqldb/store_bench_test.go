// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/sqldb"
)

func BenchmarkAttemptLock(b *testing.B) {
	for _, d := range dialects {
		b.Run(string(d), func(b *testing.B) {
			db, fake := testhelpers.NewFakeSQL(b, nil)
			fake.DisableRecording()
			s, err := sqldb.New(db, d)
			require.NoError(b, err)
			val := []byte("in-progress")
			b.ReportAllocs()
			for b.Loop() {
				_, _, _, err := s.AttemptLock(b.Context(), "key", val)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkComplete(b *testing.B) {
	benchmarkWrite(b, func(b *testing.B, s *sqldb.Storage) error {
		return s.Complete(b.Context(), "key", []byte("done"), []byte("lock"))
	})
}

func BenchmarkSteal(b *testing.B) {
	benchmarkWrite(b, func(b *testing.B, s *sqldb.Storage) error {
		_, err := s.Steal(b.Context(), "key", []byte("old"), []byte("new"))
		return err
	})
}

func BenchmarkRelease(b *testing.B) {
	benchmarkWrite(b, func(b *testing.B, s *sqldb.Storage) error {
		return s.Release(b.Context(), "key", []byte("lock"))
	})
}

func benchmarkWrite(b *testing.B, op func(*testing.B, *sqldb.Storage) error) {
	b.Helper()
	for _, d := range dialects {
		b.Run(string(d), func(b *testing.B) {
			db, fake := testhelpers.NewFakeSQL(b, nil)
			fake.DisableRecording()
			s, err := sqldb.New(db, d)
			require.NoError(b, err)
			b.ReportAllocs()
			for b.Loop() {
				if err := op(b, s); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

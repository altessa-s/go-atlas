// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/requestidstore/storages/sqldb"
)

func benchmarkStorage(b *testing.B) *sqldb.Storage {
	b.Helper()
	db, fake := testhelpers.NewFakeSQL(b, nil)
	fake.DisableRecording()
	s, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithPurgeInterval(0))
	require.NoError(b, err)
	return s
}

func BenchmarkReserve(b *testing.B) {
	s := benchmarkStorage(b)
	now := time.Now()
	p := requestid.Entry{State: requestid.StatePending, Token: "owner", ExpiresAt: now.Add(time.Minute)}
	b.ReportAllocs()
	for b.Loop() {
		_, _, err := s.Reserve(b.Context(), "key", p, now)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkComplete(b *testing.B) {
	s := benchmarkStorage(b)
	now := time.Now()
	done := requestid.Entry{State: requestid.StateCompleted, Token: "owner", ExpiresAt: now.Add(time.Hour)}
	b.ReportAllocs()
	for b.Loop() {
		if err := s.Complete(b.Context(), "key", "owner", done, now); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRelease(b *testing.B) {
	s := benchmarkStorage(b)
	now := time.Now()
	b.ReportAllocs()
	for b.Loop() {
		if err := s.Release(b.Context(), "key", "owner", now); err != nil {
			b.Fatal(err)
		}
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/store/mongo/mongotest"

	mongostore "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/mongo"
)

func BenchmarkReserveComplete(b *testing.B) {
	db := mongotest.New(b, mongotest.WithDatabasePrefix("requestid_bench_")).Database()
	s, err := mongostore.New(b.Context(), db)
	require.NoError(b, err)
	key := requestidtest.Key(b.Name())
	now := time.Now().UTC().Truncate(time.Millisecond)
	b.ReportAllocs()
	for b.Loop() {
		p := requestid.Entry{State: requestid.StatePending, Token: "owner", ExpiresAt: now.Add(time.Minute)}
		_, won, err := s.Reserve(b.Context(), key, p, now)
		require.NoError(b, err)
		require.True(b, won)
		p.State = requestid.StateCompleted
		require.NoError(b, s.Complete(b.Context(), key, p.Token, p, now))
		now = p.ExpiresAt
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestPurgeDisabledInterval(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "DELETE") {
			return testhelpers.FakeSQLReply{}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	s, err := New(db, DialectMySQL, WithTTL(time.Hour), WithPurgeInterval(0))
	require.NoError(t, err)
	require.NoError(t, s.Store(t.Context(), retentionEvent("key")))
	n, err := s.PurgeExpired(t.Context())
	require.NoError(t, err)
	require.Zero(t, n)
	require.NotEmpty(t, fake.Calls(), "manual purge remains available")
	require.NoError(t, s.Close(t.Context()))
	require.Len(t, fake.Calls(), 2, "only the write and explicit purge run")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, s.Close(ctx), "an absent loop satisfies an ended context")
}

func TestPostgresPurgeSkipsLockedRows(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	s, err := New(db, DialectPostgres, WithTTL(time.Hour))
	require.NoError(t, err)
	_, err = s.PurgeExpired(t.Context())
	require.NoError(t, err)
	require.Contains(t, fake.Calls()[0].Query, "ORDER BY ts_ms, id LIMIT $2 FOR UPDATE SKIP LOCKED)")
}

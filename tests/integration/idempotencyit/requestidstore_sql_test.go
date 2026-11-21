// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/reliability/requestidstore/storages/sqldb"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"
)

func TestRequestIDSQL(t *testing.T) {
	t.Parallel()
	for _, target := range livebackend.SQLTargets() {
		t.Run(target.Name, func(t *testing.T) {
			t.Parallel()
			db := target.Open(t)
			fresh := func(t *testing.T) *sqldb.Storage {
				t.Helper()
				table := livebackend.SQLTable(t, db, "reqid")
				s, err := sqldb.New(db, sqldb.Dialect(target.Dialect), sqldb.WithTableName(table), sqldb.WithPurgeInterval(0), sqldb.WithPurgeBatchSize(1))
				require.NoError(t, err)
				require.NoError(t, s.EnsureSchema(t.Context()))
				t.Cleanup(func() { require.NoError(t, s.Close(context.WithoutCancel(t.Context()))) })
				return s
			}
			requestidtest.Run(t, func(t *testing.T) requestid.Store { return fresh(t) })
			t.Run("PurgeCallerTime", func(t *testing.T) {
				t.Parallel()
				s := fresh(t)
				now := requestidtest.Base
				p := requestid.Entry{State: requestid.StatePending, Token: "owner", ExpiresAt: now.Add(time.Second)}
				for _, key := range []string{"expired", "also-expired", "live"} {
					if key == "live" {
						p.ExpiresAt = now.Add(time.Hour)
					}
					_, won, err := s.Reserve(t.Context(), requestidtest.Key(key), p, now)
					require.NoError(t, err)
					require.True(t, won)
				}
				n, err := s.PurgeExpired(t.Context())
				require.NoError(t, err)
				require.Zero(t, n)
				at := now.Add(time.Second)
				_, won, err := s.Reserve(t.Context(), requestidtest.Key("live"), p, at)
				require.NoError(t, err)
				require.False(t, won)
				n, err = s.PurgeExpired(t.Context())
				require.NoError(t, err)
				require.EqualValues(t, 2, n)
				require.NoError(t, s.Release(t.Context(), requestidtest.Key("live"), "owner", at))
			})
		})
	}
}

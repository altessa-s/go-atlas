// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/requestidstore/storages/sqldb"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

func TestNew(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	_, err := sqldb.New(nil, sqldb.DialectMySQL)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	_, err = sqldb.New(db, "other")
	require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTableName("broken name"))
	require.ErrorIs(t, err, sqldb.ErrInvalidTableName)
}

func TestOwnershipPredicate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                          string
		dialect                       sqldb.Dialect
		releaseFilter, completeFilter string
	}{
		{"postgres", sqldb.DialectPostgres, "request_key = $1 AND state = $2 AND token = $3 AND expires_at > $4", "request_key = $8 AND state = $9 AND token = $10 AND expires_at > $11"},
		{"mysql", sqldb.DialectMySQL, "request_key = ? AND state = ? AND token = ? AND expires_at > ?", "request_key = ? AND state = ? AND token = ? AND expires_at > ?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{} })
			s, err := sqldb.New(db, tc.dialect, sqldb.WithPurgeInterval(0))
			require.NoError(t, err)
			key := requestidtest.Key(t.Name())
			now := requestidtest.Base
			token := "caller\x00token"
			done := requestid.Entry{State: requestid.StateCompleted, Token: "new", Response: []byte("outcome"), ExpiresAt: now.Add(time.Hour)}
			require.ErrorIs(t, s.Complete(t.Context(), key, token, done, now), requestid.ErrReservationLost)
			require.ErrorIs(t, s.Release(t.Context(), key, token, now), requestid.ErrReservationLost)
			calls := fake.Calls()
			ownerArgs := []any{key, int64(requestid.StatePending), []byte(token), now.UnixMilli()}
			require.Contains(t, calls[0].Query, tc.completeFilter)
			require.Equal(t, ownerArgs, calls[0].Args[7:])
			require.Equal(t, done.ExpiresAt.UnixMilli(), calls[0].Args[6])
			require.Contains(t, calls[1].Query, tc.releaseFilter)
			require.Equal(t, ownerArgs, calls[1].Args)
		})
	}
}

func TestReserveRacesRelease(t *testing.T) {
	t.Parallel()
	inserts := 0
	db, fake := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		switch {
		case strings.HasPrefix(query, "INSERT"):
			inserts++
			if inserts == 2 {
				return testhelpers.FakeSQLReply{Affected: 1}
			}
		case strings.HasPrefix(query, "SELECT"):
			return testhelpers.FakeSQLReply{Columns: []string{"state", "token", "fingerprint", "response", "failure", "response_omitted", "expires_at"}, Rows: [][]driver.Value{}}
		}
		return testhelpers.FakeSQLReply{}
	})
	s, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithPurgeInterval(0))
	require.NoError(t, err)
	now := requestidtest.Base
	p := requestid.Entry{State: requestid.StatePending, Token: "a", ExpiresAt: now.Add(time.Minute)}
	got, won, err := s.Reserve(t.Context(), requestidtest.Key(t.Name()), p, now)
	require.NoError(t, err)
	require.True(t, won)
	require.Equal(t, p, got)
	calls := fake.Calls()
	require.Len(t, calls, 4)
	require.Contains(t, calls[2].Query, "expires_at <= $9")
	require.Equal(t, now.UnixMilli(), calls[2].Args[8])
}

func TestPurgeSkew(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	s, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithPurgeInterval(0))
	require.NoError(t, err)

	// A release on an unrelated key advances this replica's cleanup clock.
	now := requestidtest.Base
	leading := now.Add(5 * time.Minute)
	require.NoError(t, s.Release(t.Context(), "other", "token", leading))
	_, err = s.PurgeExpired(t.Context())
	require.NoError(t, err)
	calls := fake.Calls()
	require.Len(t, calls, 2)
	require.Contains(t, calls[1].Query, "expires_at <= $1")
	require.Equal(t, leading.Add(-sqldb.DefaultCleanupGrace).UnixMilli(), calls[1].Args[0])
}

func TestCanceledOrFailedWrite(t *testing.T) {
	t.Parallel()
	serverErr := errors.New("unavailable")
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Err: serverErr} })
	s, err := sqldb.New(db, sqldb.DialectMySQL, sqldb.WithPurgeInterval(0))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, s.Release(ctx, "key", "token", requestidtest.Base), context.Canceled)
	require.Empty(t, fake.Calls())
	require.ErrorIs(t, s.Release(t.Context(), "key", "token", requestidtest.Base), serverErr)
}

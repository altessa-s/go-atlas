// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/outbox"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	outboxsql "github.com/altessa-s/go-atlas/data/outbox/store/sqldb"
)

// Use the internal assembly boundary to test registration without a database.
func TestBuildPropagatesRegistrationFailureAndRetainsRetryableOutbox(t *testing.T) {
	t.Parallel()
	boom := errors.New("registrar unavailable")
	reg := &testhelpers.MockTaskRegistrar{Err: boom}
	b := New(&config.Outbox{Enabled: true, DispatchSchedule: "@every 1m",
		DispatchTaskID: "dispatch", UnlockTaskID: "unlock", ExpireTaskID: "expire", CleanupTaskID: "cleanup", StatsTaskID: "stats",
	}).UseScheduler(reg)
	ob, err := b.createOutboxWithStore(nil, nil)
	require.ErrorIs(t, err, boom)
	require.NotNil(t, ob)
	reg.Err = nil
	require.NoError(t, ob.RegisterTasks(t.Context()))
}

func TestBuildWithSQLDB(t *testing.T) {
	t.Parallel()
	cfg := &config.Outbox{Enabled: true}

	_, err := New(cfg).BuildWithSQLDB(nil, outboxsql.DialectPostgres, nil)
	require.ErrorContains(t, err, "SQL database")

	db, fake := testhelpers.NewFakeSQL(t, nil)
	_, err = New(cfg).BuildWithSQLDB(db, "oracle", nil)
	require.ErrorIs(t, err, outboxsql.ErrUnsupportedDialect)

	handler := func(context.Context, outbox.Event) error { return nil }
	ob, err := New(cfg).BuildWithSQLDB(db, outboxsql.DialectMySQL, handler)
	require.NoError(t, err)
	require.NotNil(t, ob)
	require.Empty(t, fake.Calls(), "without ensureSchema the build performs no I/O")
}

func TestBuildWithSQLDBEnsureSchema(t *testing.T) {
	t.Parallel()
	handler := func(context.Context, outbox.Event) error { return nil }

	t.Run("creates_schema", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, nil)
		ob, err := New(&config.Outbox{Enabled: true, EnsureSchema: true}).BuildWithSQLDB(db, outboxsql.DialectPostgres, handler)
		require.NoError(t, err)
		require.NotNil(t, ob)
		calls := fake.Calls()
		require.Len(t, calls, 5, "advisory lock, table, three indexes")
		require.Contains(t, calls[1].Query, "CREATE TABLE IF NOT EXISTS")
	})

	t.Run("disabled_outbox_runs_no_ddl", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, nil)
		ob, err := New(&config.Outbox{EnsureSchema: true}).BuildWithSQLDB(db, outboxsql.DialectPostgres, handler)
		require.NoError(t, err)
		require.Nil(t, ob)
		require.Empty(t, fake.Calls())
	})

	t.Run("propagates_failure", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("ddl denied")
		db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Err: boom} })
		_, err := New(&config.Outbox{Enabled: true, EnsureSchema: true}).BuildWithSQLDB(db, outboxsql.DialectMySQL, handler)
		require.ErrorIs(t, err, boom)
	})
}

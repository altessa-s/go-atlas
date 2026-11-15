// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

// Use the internal assembly boundary to test registration without a database.
func TestBuildPropagatesRegistrationFailureAndRetainsRetryableOutbox(t *testing.T) {
	t.Parallel()
	boom := errors.New("registrar unavailable")
	reg := &testhelpers.MockTaskRegistrar{Err: boom}
	b := New(&brokerconfig.Outbox{Enabled: true, DispatchSchedule: "@every 1m",
		DispatchTaskID: "dispatch", UnlockTaskID: "unlock", ExpireTaskID: "expire", CleanupTaskID: "cleanup", StatsTaskID: "stats",
	}).UseScheduler(reg)
	ob, err := b.createOutboxWithStore(t.Context(), nil, nil)
	require.ErrorIs(t, err, boom)
	require.NotNil(t, ob)
	reg.Err = nil
	require.NoError(t, ob.RegisterTasks(t.Context()))
}

func TestBuildWithSQLDB(t *testing.T) {
	t.Parallel()
	cfg := &brokerconfig.Outbox{Enabled: true}

	_, err := New(cfg).BuildWithSQLDB(t.Context(), nil, outboxsql.DialectPostgres, nil)
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)

	db, fake := testhelpers.NewFakeSQL(t, nil)
	_, err = New(cfg).BuildWithSQLDB(t.Context(), db, "oracle", nil)
	require.ErrorIs(t, err, outboxsql.ErrUnsupportedDialect)

	handler := func(context.Context, outbox.Event) error { return nil }
	ob, err := New(cfg).BuildWithSQLDB(t.Context(), db, outboxsql.DialectMySQL, handler)
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
		ob, err := New(&brokerconfig.Outbox{Enabled: true, EnsureSchema: true}).BuildWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, handler)
		require.NoError(t, err)
		require.NotNil(t, ob)
		calls := fake.Calls()
		require.Len(t, calls, 6, "advisory lock, table, four indexes")
		require.Contains(t, calls[1].Query, "CREATE TABLE IF NOT EXISTS")
	})

	t.Run("disabled_outbox_runs_no_ddl", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, nil)
		ob, err := New(&brokerconfig.Outbox{EnsureSchema: true}).BuildWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, handler)
		require.NoError(t, err)
		require.Nil(t, ob)
		require.Empty(t, fake.Calls())
	})

	t.Run("propagates_failure", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("ddl denied")
		db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Err: boom} })
		_, err := New(&brokerconfig.Outbox{Enabled: true, EnsureSchema: true}).BuildWithSQLDB(t.Context(), db, outboxsql.DialectMySQL, handler)
		require.ErrorIs(t, err, boom)
	})
}

// Code-supplied options apply after the configuration, so they win on overlap.
func TestUseOutboxOptionsOverridesConfig(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	handler := func(context.Context, outbox.Event) error { return nil }

	ob, err := New(&brokerconfig.Outbox{Enabled: true, MaxPayloadBytes: 1024}).
		UseOutboxOptions(outbox.WithMaxPayloadBytes(8)).
		BuildWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, handler)
	require.NoError(t, err)

	err = ob.Save(t.Context(), outbox.Event{Key: "k", Payload: make([]byte, 16)})
	require.ErrorIs(t, err, outbox.ErrPayloadTooLarge)
}

// The caller's context bounds the startup schema creation: a canceled startup
// must not run DDL on a detached context.
func TestBuildWithSQLDBHonorsContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	db, fake := testhelpers.NewFakeSQL(t, nil)
	handler := func(context.Context, outbox.Event) error { return nil }
	_, err := New(&brokerconfig.Outbox{Enabled: true, EnsureSchema: true}).
		BuildWithSQLDB(ctx, db, outboxsql.DialectPostgres, handler)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, fake.Calls())
}

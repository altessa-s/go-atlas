// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// sqlConfig returns an enabled outbox config over the sqldb storage.
func sqlConfig(dialect string, ensureSchema bool) *brokerconfig.Outbox {
	return &brokerconfig.Outbox{
		Enabled: true,
		Storage: &brokerconfig.OutboxStorageConfig{
			Type: brokerconfig.OutboxStorageTypeSQL,
			SQL:  &brokerconfig.OutboxSQLStorageConfig{Dialect: dialect, Table: "events_outbox", EnsureSchema: ensureSchema},
		},
	}
}

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

func TestBuildSQLDB(t *testing.T) {
	t.Parallel()

	_, err := New(sqlConfig(brokerconfig.SQLDialectPostgres, false)).Build(t.Context(), noopHandler)
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)

	db, fake := testhelpers.NewFakeSQL(t, nil)
	_, err = New(sqlConfig("oracle", false)).UseSQLDB(db).Build(t.Context(), noopHandler)
	_, isValidation := errors.AsType[validation.Errors](err)
	require.True(t, isValidation, "the config rejects an unknown dialect: %v", err)

	ob, err := New(sqlConfig(brokerconfig.SQLDialectMySQL, false)).UseSQLDB(db).Build(t.Context(), noopHandler)
	require.NoError(t, err)
	require.NotNil(t, ob)
	require.Empty(t, fake.Calls(), "without ensureSchema the build performs no I/O")
}

func TestBuildSQLDBEnsureSchema(t *testing.T) {
	t.Parallel()

	t.Run("creates_schema", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, nil)
		ob, err := New(sqlConfig(brokerconfig.SQLDialectPostgres, true)).UseSQLDB(db).Build(t.Context(), noopHandler)
		require.NoError(t, err)
		require.NotNil(t, ob)
		calls := fake.Calls()
		require.Len(t, calls, 6, "advisory lock, table, four indexes")
		require.Contains(t, calls[1].Query, "CREATE TABLE IF NOT EXISTS")
	})

	t.Run("disabled_outbox_runs_no_ddl", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, nil)
		cfg := sqlConfig(brokerconfig.SQLDialectPostgres, true)
		cfg.Enabled = false
		ob, err := New(cfg).UseSQLDB(db).Build(t.Context(), noopHandler)
		require.NoError(t, err)
		require.Nil(t, ob)
		require.Empty(t, fake.Calls())
	})

	t.Run("propagates_failure", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("ddl denied")
		db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Err: boom} })
		_, err := New(sqlConfig(brokerconfig.SQLDialectMySQL, true)).UseSQLDB(db).Build(t.Context(), noopHandler)
		require.ErrorIs(t, err, boom)
	})
}

// Code-supplied options apply after the configuration, so they win on overlap.
func TestUseOutboxOptionsOverridesConfig(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig(brokerconfig.SQLDialectPostgres, false)
	cfg.MaxPayloadBytes = 1024

	ob, err := New(cfg).UseSQLDB(db).UseOutboxOptions(outbox.WithMaxPayloadBytes(8)).Build(t.Context(), noopHandler)
	require.NoError(t, err)

	err = ob.Save(t.Context(), outbox.Event{Key: "k", Payload: make([]byte, 16)})
	require.ErrorIs(t, err, outbox.ErrPayloadTooLarge)
}

// The caller's context bounds the startup schema creation: a canceled startup
// must not run DDL on a detached context.
func TestBuildSQLDBHonorsContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	db, fake := testhelpers.NewFakeSQL(t, nil)
	_, err := New(sqlConfig(brokerconfig.SQLDialectPostgres, true)).UseSQLDB(db).Build(ctx, noopHandler)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, fake.Calls())
}

func TestBuildWatchValidation(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)

	cfg := sqlConfig(brokerconfig.SQLDialectMySQL, false)
	cfg.Watch = brokerconfig.OutboxWatchNotify
	_, err := New(cfg).UseSQLDB(db).UseSQLListener(&fakeListener{}).Build(t.Context(), noopHandler)
	require.ErrorIs(t, err, brokerconfig.ErrUnsupportedOutboxWatch)

	cfg = sqlConfig(brokerconfig.SQLDialectPostgres, false)
	cfg.Watch = brokerconfig.OutboxWatchChangeStream
	_, err = New(cfg).UseSQLDB(db).Build(t.Context(), noopHandler)
	require.ErrorIs(t, err, brokerconfig.ErrUnsupportedOutboxWatch)

	cfg = sqlConfig(brokerconfig.SQLDialectPostgres, false)
	cfg.Watch = brokerconfig.OutboxWatchNotify
	var typedNil *fakeListener
	_, err = New(cfg).UseSQLDB(db).UseSQLListener(typedNil).Build(t.Context(), noopHandler)
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired, "notify needs a listener")
}

// fakeListener records subscriptions and keeps each open until its context ends.
type fakeListener struct {
	mu       sync.Mutex
	channels []string
}

func (l *fakeListener) Listen(ctx context.Context, channel string) (<-chan struct{}, error) {
	l.mu.Lock()
	l.channels = append(l.channels, channel)
	l.mu.Unlock()
	ch := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (l *fakeListener) subscriptions() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.channels...)
}

// With watch: notify the store signals on save, and the builder runs the
// watcher until its shutdown hook stops it.
func TestBuildNotifyStartsAndStopsTheWatcher(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	listener := &fakeListener{}
	var hooks coreruntime.HookGroup
	cfg := sqlConfig(brokerconfig.SQLDialectPostgres, false)
	cfg.Watch = brokerconfig.OutboxWatchNotify

	ob, err := New(cfg).UseSQLDB(db).UseSQLListener(listener).UseShutdownHooks(&hooks).Build(t.Context(), noopHandler)
	require.NoError(t, err)

	testhelpers.WaitFor(t, 5*time.Second, func() bool { return len(listener.subscriptions()) == 1 }, "the watcher subscribes")
	require.Equal(t, []string{"events_outbox"}, listener.subscriptions())

	require.NoError(t, ob.Save(t.Context(), outbox.Event{Key: "k", Payload: []byte("x")}))
	calls := fake.Calls()
	require.Equal(t, "SELECT pg_notify($1, '')", calls[len(calls)-1].Query)

	require.NoError(t, hooks.Shutdown(t.Context()))
}

func TestBuildMongoRequiresRegistration(t *testing.T) {
	t.Parallel()
	_, err := New(&brokerconfig.Outbox{Enabled: true}).Build(t.Context(), noopHandler)
	require.ErrorIs(t, err, corefactory.ErrNotRegistered)
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
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
		require.Contains(t, calls[0].Query, "pg_advisory_xact_lock")
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
	cfg.DispatchSchedule = "@every 1m"
	_, err := New(cfg).UseSQLDB(db).UseSQLListener(&fakeListener{}).Build(t.Context(), noopHandler)
	require.ErrorIs(t, err, brokerconfig.ErrUnsupportedOutboxWatch)

	cfg = sqlConfig(brokerconfig.SQLDialectPostgres, false)
	cfg.Watch = brokerconfig.OutboxWatchChangeStream
	_, err = New(cfg).UseSQLDB(db).Build(t.Context(), noopHandler)
	require.ErrorIs(t, err, brokerconfig.ErrUnsupportedOutboxWatch)

	cfg = sqlConfig(brokerconfig.SQLDialectPostgres, false)
	cfg.Watch = brokerconfig.OutboxWatchNotify
	cfg.DispatchSchedule = "@every 1m"
	var typedNil *fakeListener
	_, err = New(cfg).UseSQLDB(db).UseSQLListener(typedNil).UseScheduler(&testhelpers.MockTaskRegistrar{}).Build(t.Context(), noopHandler)
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired, "notify needs a listener")
}

// fakeListener records subscriptions and keeps each open until its context ends.
type fakeListener struct {
	mu       sync.Mutex
	channels []string
	stopped  chan struct{}
}

func (l *fakeListener) Listen(ctx context.Context, channel string) (<-chan struct{}, error) {
	l.mu.Lock()
	l.channels = append(l.channels, channel)
	l.mu.Unlock()
	ch := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(ch)
		if l.stopped != nil {
			close(l.stopped)
		}
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
	for _, registrationError := range []error{nil, errStorage} {
		for _, channel := range []string{"", "CustomWake"} {
			t.Run(fmt.Sprintf("registration=%v/channel=%s", registrationError, channel), func(t *testing.T) {
				t.Parallel()
				db, fake := testhelpers.NewFakeSQL(t, nil)
				listener := &fakeListener{stopped: make(chan struct{})}
				var hooks coreruntime.HookGroup
				t.Cleanup(func() { require.NoError(t, hooks.Shutdown(context.WithoutCancel(t.Context()))) })
				cfg := sqlConfig(brokerconfig.SQLDialectPostgres, false)
				cfg.Watch = brokerconfig.OutboxWatchNotify
				cfg.DispatchSchedule = "@every 1m"
				cfg.Storage.SQL.NotifyChannel = channel
				reg := &testhelpers.MockTaskRegistrar{Err: registrationError}

				ob, err := New(cfg).UseSQLDB(db).UseSQLListener(listener).UseScheduler(reg).UseShutdownHooks(&hooks).Build(t.Context(), noopHandler)
				if registrationError != nil {
					require.ErrorIs(t, err, registrationError)
					require.NoError(t, hooks.Shutdown(t.Context()))
					require.Empty(t, listener.subscriptions(), "a failed build leaves no watcher")
					reg.Err = nil
					require.NoError(t, ob.RegisterTasks(t.Context()))
					ob.StartWatch(t.Context())
					t.Cleanup(ob.StopWatch)
				} else {
					require.NoError(t, err)
				}
				require.NotNil(t, ob)
				testhelpers.WaitFor(t, 5*time.Second, func() bool { return len(listener.subscriptions()) == 1 }, "the watcher subscribes")
				wantChannel := channel
				if wantChannel == "" {
					wantChannel = "events_outbox"
				}
				require.Equal(t, []string{wantChannel}, listener.subscriptions())

				require.NoError(t, ob.Save(t.Context(), outbox.Event{Key: "k", Payload: []byte("x")}))
				calls := fake.Calls()
				require.Len(t, calls, 1)
				require.Contains(t, calls[0].Query, "SELECT pg_notify(")
				require.Equal(t, wantChannel, calls[0].Args[len(calls[0].Args)-1])
				if registrationError != nil {
					ob.StopWatch()
				}
				require.NoError(t, hooks.Shutdown(t.Context()))
				select {
				case <-listener.stopped:
				case <-time.After(5 * time.Second):
					require.FailNow(t, "the shutdown hook did not stop the listener")
				}
				require.Len(t, listener.subscriptions(), 1, "retrying registration does not start a second watcher")
			})
		}
	}
}

func TestBuildMongoRequiresRegistration(t *testing.T) {
	t.Parallel()
	_, err := New(&brokerconfig.Outbox{Enabled: true}).Build(t.Context(), noopHandler)
	require.ErrorIs(t, err, corefactory.ErrNotRegistered)
}

func TestBuildWatchRequiresSchedulerBeforeStorage(t *testing.T) {
	t.Parallel()
	for _, watch := range []brokerconfig.OutboxWatch{brokerconfig.OutboxWatchChangeStream, brokerconfig.OutboxWatchNotify} {
		t.Run(string(watch), func(t *testing.T) {
			t.Parallel()
			cfg := &brokerconfig.Outbox{Enabled: true, Watch: watch, DispatchSchedule: "@every 1m"}
			if watch == brokerconfig.OutboxWatchNotify {
				cfg = sqlConfig(brokerconfig.SQLDialectPostgres, true)
				cfg.Watch, cfg.DispatchSchedule = watch, "@every 1m"
			}
			db, fake := testhelpers.NewFakeSQL(t, nil)
			built := false
			build := func(context.Context, *brokerconfig.Outbox, *slog.Logger) (outbox.Storage, error) {
				built = true
				return nil, errStorage
			}
			ob, err := New(cfg).UseSQLDB(db).UseSQLListener(&fakeListener{}).
				UseStorage(brokerconfig.OutboxStorageTypeMongo, build).Build(t.Context(), noopHandler)
			require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
			require.Nil(t, ob)
			require.False(t, built)
			require.Empty(t, fake.Calls(), "the scheduler is checked before schema creation")
		})
	}
}

func TestBuildDoesNotNormalizeCallerConfig(t *testing.T) {
	t.Parallel()
	for _, storage := range []*brokerconfig.OutboxStorageConfig{nil, {Type: brokerconfig.OutboxStorageTypeSQL}} {
		t.Run(fmt.Sprintf("storage=%v", storage), func(t *testing.T) {
			t.Parallel()
			cfg := &brokerconfig.Outbox{Enabled: true, Storage: storage}
			before := *cfg
			var storageBefore brokerconfig.OutboxStorageConfig
			if storage != nil {
				storageBefore = *storage
			}
			db, _ := testhelpers.NewFakeSQL(t, nil)
			store, err := outboxsql.New(db, outboxsql.DialectPostgres)
			require.NoError(t, err)
			build := func(context.Context, *brokerconfig.Outbox, *slog.Logger) (outbox.Storage, error) { return store, nil }
			ob, err := New(cfg).UseSQLDB(db).UseStorage(brokerconfig.OutboxStorageTypeMongo, build).Build(t.Context(), noopHandler)
			require.NoError(t, err)
			require.NotNil(t, ob)
			require.Equal(t, before, *cfg)
			if storage != nil {
				require.Equal(t, storageBefore, *storage)
			}
		})
	}
}

func TestBuildPollingIgnoresNotifyChannel(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig(brokerconfig.SQLDialectPostgres, false)
	cfg.Storage.SQL.NotifyChannel = "CustomWake"
	ob, err := New(cfg).UseSQLDB(db).Build(t.Context(), noopHandler)
	require.NoError(t, err)
	require.NoError(t, ob.Save(t.Context(), outbox.Event{Key: "k", Payload: []byte("x")}))
	calls := fake.Calls()
	require.Len(t, calls, 1)
	require.NotContains(t, calls[0].Query, "pg_notify")
}

type blockingWatcher struct {
	outbox.Storage
	canceled chan struct{}
	release  chan struct{}
	exited   chan struct{}
}

func (w *blockingWatcher) Watch(ctx context.Context, _ func()) error {
	<-ctx.Done()
	close(w.canceled)
	<-w.release
	close(w.exited)
	return ctx.Err()
}

func TestWatchStopJoinsWorker(t *testing.T) {
	t.Parallel()
	for _, viaHooks := range []bool{false, true} {
		t.Run(fmt.Sprintf("hooks=%t", viaHooks), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				cfg := brokerconfig.Default().Outbox
				cfg.Storage = &brokerconfig.OutboxStorageConfig{Type: "custom"}
				cfg.Watch = brokerconfig.OutboxWatchNotify
				watch := &blockingWatcher{canceled: make(chan struct{}), release: make(chan struct{}), exited: make(chan struct{})}
				var hooks coreruntime.HookGroup
				ob, err := New(&cfg).UseStorage("custom", func(context.Context, *brokerconfig.Outbox, *slog.Logger) (outbox.Storage, error) { return watch, nil }).
					UseShutdownHooks(&hooks).UseScheduler(&testhelpers.MockTaskRegistrar{}).Build(t.Context(), noopHandler)
				require.NoError(t, err)
				stopped := make(chan error, 1)
				go func() {
					if viaHooks {
						ctx, cancel := context.WithCancel(t.Context())
						cancel()
						stopped <- hooks.Shutdown(ctx)
					} else {
						ob.StopWatch()
						stopped <- nil
					}
				}()
				<-watch.canceled
				synctest.Wait()
				premature := len(stopped)
				close(watch.release)
				require.NoError(t, <-stopped)
				require.Zero(t, premature, "stop returned while storage Watch was still running")
				select {
				case <-watch.exited:
				default:
					require.FailNow(t, "storage watcher still running")
				}
				ob.StopWatch()
			})
		})
	}
}

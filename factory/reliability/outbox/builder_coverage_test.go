// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	outboxconfig "github.com/altessa-s/go-atlas/config/outbox"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func noopHandler(context.Context, outbox.Event) error { return nil }

func TestBuild_RequiresConfig(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	_, err := New(nil).UseSQLDB(db).Build(t.Context(), noopHandler)
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

func TestBuild_RegistersConfiguredSchedules(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	reg := &testhelpers.MockTaskRegistrar{}
	cfg := sqlConfig(outboxconfig.SQLDialectPostgres, false)
	cfg.TopicCompaction = true
	cfg.PublishedEventsLifetime = time.Hour
	cfg.DefaultEventTTL = time.Hour
	cfg.DispatchSchedule = "@every 1s"
	cfg.UnlockSchedule = "@every 2s"
	cfg.CleanupSchedule = "@every 3s"
	cfg.ExpireSchedule = "@every 4s"
	cfg.StatsSchedule = "@every 5s"
	cfg.DispatchTaskID, cfg.UnlockTaskID, cfg.ExpireTaskID, cfg.CleanupTaskID, cfg.StatsTaskID = "d", "u", "e", "c", "s"

	ob, err := New(cfg).
		UseLogger(slog.New(slog.DiscardHandler)).
		UseDefaultLogger().
		UseScheduler(reg).
		UseSQLDB(db).
		Build(t.Context(), noopHandler)
	require.NoError(t, err)
	require.NotNil(t, ob)

	got := map[string]string{}
	for _, task := range reg.Tasks() {
		got[task.ID] = task.Schedule
	}
	require.Equal(t, map[string]string{"d": "@every 1s", "u": "@every 2s", "c": "@every 3s", "e": "@every 4s", "s": "@every 5s"}, got)
}

var errStorage = errors.New("storage failed")

func TestUseStorage(t *testing.T) {
	t.Parallel()
	cfg := &outboxconfig.Config{Enabled: true}

	t.Run("nil_function_fails", func(t *testing.T) {
		t.Parallel()
		_, err := New(cfg).UseStorage(outboxconfig.StorageTypeMongo, nil).Build(t.Context(), noopHandler)
		require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
	})

	t.Run("function_error_propagates", func(t *testing.T) {
		t.Parallel()
		failing := func(context.Context, *outboxconfig.Config, *slog.Logger) (outbox.Storage, error) {
			return nil, errStorage
		}
		_, err := New(&outboxconfig.Config{Enabled: true}).UseStorage(outboxconfig.StorageTypeMongo, failing).Build(t.Context(), noopHandler)
		require.ErrorIs(t, err, errStorage)
	})

	t.Run("function_storage_used", func(t *testing.T) {
		t.Parallel()
		db, _ := testhelpers.NewFakeSQL(t, nil)
		want, err := outboxsql.New(db, outboxsql.DialectPostgres)
		require.NoError(t, err)
		local := &outboxconfig.Config{Enabled: true}
		build := func(ctx context.Context, got *outboxconfig.Config, _ *slog.Logger) (outbox.Storage, error) {
			require.NotSame(t, local, got)
			require.Equal(t, "events_outbox", got.Storage.Mongo.Collection, "the builder normalizes the storage section")
			require.NotNil(t, ctx)
			return want, nil
		}
		ob, err := New(local).UseStorage(outboxconfig.StorageTypeMongo, build).Build(t.Context(), noopHandler)
		require.NoError(t, err)
		require.NotNil(t, ob)
	})

	t.Run("change_stream_starts_the_watcher", func(t *testing.T) {
		t.Parallel()
		db, _ := testhelpers.NewFakeSQL(t, nil)
		store, err := outboxsql.New(db, outboxsql.DialectPostgres) // No listener: Watch is unsupported.
		require.NoError(t, err)
		local := &outboxconfig.Config{Enabled: true, Watch: outboxconfig.WatchChangeStream, DispatchSchedule: "@every 1m"}
		build := func(context.Context, *outboxconfig.Config, *slog.Logger) (outbox.Storage, error) { return store, nil }

		var hooks coreruntime.HookGroup
		ob, err := New(local).UseScheduler(&testhelpers.MockTaskRegistrar{}).UseStorage(outboxconfig.StorageTypeMongo, build).UseShutdownHooks(&hooks).Build(t.Context(), noopHandler)
		require.NoError(t, err)
		require.NotNil(t, ob)
		require.NoError(t, hooks.Shutdown(t.Context()), "a watcher that reported unsupported has already returned")
	})
}

type pollingStorage struct{ outbox.Storage }

type watchStorage struct {
	outbox.Storage
	started chan struct{}
}

func (s *watchStorage) Watch(ctx context.Context, _ func()) error {
	if s == nil {
		return outbox.ErrWatchUnsupported
	}
	close(s.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestBuildCustomBackend(t *testing.T) {
	t.Parallel()
	cfg := outboxconfig.Default()
	cfg.Storage = &outboxconfig.Storage{Type: "custom"}
	cfg.Watch = outboxconfig.WatchNotify

	reg := &testhelpers.MockTaskRegistrar{}
	var hooks coreruntime.HookGroup
	t.Cleanup(func() { require.NoError(t, hooks.Shutdown(context.WithoutCancel(t.Context()))) })
	b := New(&cfg).UseScheduler(reg).UseShutdownHooks(&hooks)
	_, err := b.Build(t.Context(), noopHandler)
	var fields validation.Errors
	require.ErrorAs(t, err, &fields)
	require.ErrorIs(t, err, outboxconfig.ErrUnsupportedWatch)
	store := &watchStorage{started: make(chan struct{})}
	b.UseStorage("custom", func(context.Context, *outboxconfig.Config, *slog.Logger) (outbox.Storage, error) { return store, nil })
	cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())
	require.NoError(t, cfg.Validate())
	cfg.Storage.SetRegisteredTypes(nil)
	ob, err := b.Build(t.Context(), noopHandler)
	require.NoError(t, err)
	require.NotNil(t, ob)
	select {
	case <-store.started:
	case <-time.After(3 * time.Second):
		require.FailNow(t, "registered watcher did not start")
	}
}

func TestBuildCustomBackendWithoutWatcher(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		store outbox.Storage
	}{
		{"polling only", pollingStorage{}},
		{"nil watcher", (*watchStorage)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := outboxconfig.Default()
			cfg.Storage = &outboxconfig.Storage{Type: "custom"}
			cfg.Watch = outboxconfig.WatchChangeStream
			b := New(&cfg).UseScheduler(&testhelpers.MockTaskRegistrar{})
			b.UseStorage("custom", func(context.Context, *outboxconfig.Config, *slog.Logger) (outbox.Storage, error) {
				return tc.store, nil
			})
			ob, err := b.Build(t.Context(), noopHandler)
			require.ErrorIs(t, err, outboxconfig.ErrUnsupportedWatch)
			require.Nil(t, ob)
		})
	}
}

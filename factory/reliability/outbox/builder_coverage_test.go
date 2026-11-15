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

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

func noopHandler(context.Context, outbox.Event) error { return nil }

func TestBuildWithSQLDB_RequiresConfig(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	_, err := New(nil).BuildWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, noopHandler)
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

func TestBuildWithSQLDB_RegistersConfiguredSchedules(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	reg := &testhelpers.MockTaskRegistrar{}
	cfg := &brokerconfig.Outbox{
		Enabled:                 true,
		TopicCompaction:         true,
		PublishedEventsLifetime: time.Hour,
		DefaultEventTTL:         time.Hour,
		DispatchSchedule:        "@every 1s",
		UnlockSchedule:          "@every 2s",
		CleanupSchedule:         "@every 3s",
		ExpireSchedule:          "@every 4s",
		StatsSchedule:           "@every 5s",
		DispatchTaskID:          "d", UnlockTaskID: "u", ExpireTaskID: "e", CleanupTaskID: "c", StatsTaskID: "s",
	}
	ob, err := New(cfg).
		UseLogger(slog.New(slog.DiscardHandler)).
		UseDefaultLogger().
		UseScheduler(reg).
		BuildWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, noopHandler)
	require.NoError(t, err)
	require.NotNil(t, ob)

	got := map[string]string{}
	for _, task := range reg.Tasks() {
		got[task.ID] = task.Schedule
	}
	require.Equal(t, map[string]string{"d": "@every 1s", "u": "@every 2s", "c": "@every 3s", "e": "@every 4s", "s": "@every 5s"}, got)
}

var errStorage = errors.New("storage failed")

func TestBuildWithStorage(t *testing.T) {
	t.Parallel()
	cfg := &brokerconfig.Outbox{Enabled: true}

	t.Run("nil_function_fails", func(t *testing.T) {
		t.Parallel()
		_, err := New(cfg).BuildWithStorage(t.Context(), nil, noopHandler)
		require.Error(t, err)
	})

	t.Run("function_error_propagates", func(t *testing.T) {
		t.Parallel()
		failing := func(context.Context, *brokerconfig.Outbox, *slog.Logger) (outbox.Storage, error) { return nil, errStorage }
		_, err := New(cfg).BuildWithStorage(t.Context(), failing, noopHandler)
		require.ErrorIs(t, err, errStorage)
	})

	t.Run("function_storage_used", func(t *testing.T) {
		t.Parallel()
		db, _ := testhelpers.NewFakeSQL(t, nil)
		want, err := outboxsql.New(db, outboxsql.DialectPostgres)
		require.NoError(t, err)
		build := func(ctx context.Context, got *brokerconfig.Outbox, _ *slog.Logger) (outbox.Storage, error) {
			require.Same(t, cfg, got)
			require.NotNil(t, ctx)
			return want, nil
		}
		ob, err := New(cfg).BuildWithStorage(t.Context(), build, noopHandler)
		require.NoError(t, err)
		require.NotNil(t, ob)
	})
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerit_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/storagetest"

	chstore "github.com/altessa-s/go-atlas/service/scheduler/storages/clickhouse"
)

// clickHouseFixtureTTL outlives the contract fixtures, whose timestamps sit a
// few seconds after the Unix epoch: ClickHouse drops a row already past its
// TTL as soon as it is written. It stays short enough for a current timestamp
// plus the TTL to fit DateTime, which ends in 2106.
const clickHouseFixtureTTL = 70 * 365 * 24 * time.Hour

// openClickHouse connects to the ClickHouse of the docker stack, skipping when
// it is unreachable.
func openClickHouse(tb testing.TB) driver.Conn {
	tb.Helper()
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{envOr("CLICKHOUSE_ADDR", "127.0.0.1:19001")},
		Auth: clickhouse.Auth{
			Database: envOr("CLICKHOUSE_DB", "default"),
			Username: envOr("CLICKHOUSE_USER", "atlas"),
			Password: envOr("CLICKHOUSE_PASSWORD", "atlas"),
		},
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		tb.Skipf("clickhouse not available (%v) — start it with: make integration-up", err)
	}
	if err := conn.Ping(tb.Context()); err != nil {
		_ = conn.Close()
		tb.Skipf("clickhouse unreachable (%v) — start it with: make integration-up", err)
	}
	tb.Cleanup(func() { _ = conn.Close() })
	return conn
}

// newClickHouseHistory returns a history storage over a throwaway table,
// created through EnsureSchema (twice, to pin its idempotence) and dropped on
// cleanup.
func newClickHouseHistory(tb testing.TB, opts ...chstore.Option) (*chstore.Storage, driver.Conn, string) {
	tb.Helper()
	conn := openClickHouse(tb)
	table := sqlTableName(tb, "history")
	tb.Cleanup(func() {
		_ = conn.Exec(context.WithoutCancel(tb.Context()), "DROP TABLE IF EXISTS `"+table+"`")
	})
	opts = append([]chstore.Option{chstore.WithTableName(table), chstore.WithTTL(clickHouseFixtureTTL)}, opts...)
	store, err := chstore.New(conn, opts...)
	require.NoError(tb, err)
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	return store, conn, table
}

// TestClickHouseHistoryContract runs the history storage contract suite on
// ClickHouse, each contract over its own table.
func TestClickHouseHistoryContract(t *testing.T) {
	t.Parallel()
	storagetest.RunHistory(t, func(tb testing.TB) scheduler.HistoryStorage {
		store, _, _ := newClickHouseHistory(tb)
		return store
	})
}

// TestClickHouseHistoryTTL pins that retention is the table TTL: an entry that
// ended longer ago than the TTL is gone once the parts merge, a recent one
// stays, and CleanupHistory itself deletes nothing.
func TestClickHouseHistoryTTL(t *testing.T) {
	t.Parallel()
	store, conn, table := newClickHouseHistory(t, chstore.WithTTL(time.Hour))
	ctx := t.Context()
	now := time.Now().Unix()
	for id, endedAt := range map[string]int64{"old": now - 7200, "recent": now - 60} {
		require.NoError(t, store.AddHistory(ctx, &scheduler.TaskHistory{ID: id, TaskID: "ttl", StartedAt: endedAt - 1, EndedAt: endedAt}))
	}

	require.NoError(t, store.CleanupHistory(ctx, time.Nanosecond))
	require.NoError(t, conn.Exec(ctx, "OPTIMIZE TABLE `"+table+"` FINAL"))

	var ids []string
	for h, err := range store.History(ctx, "ttl") {
		require.NoError(t, err)
		ids = append(ids, h.ID)
	}
	require.Equal(t, []string{"recent"}, ids)
}

// TestClickHouseHistoryWithScheduler runs a task on a scheduler whose history
// lives in ClickHouse: the run is listed through the scheduler, and Unregister
// deletes it.
func TestClickHouseHistoryWithScheduler(t *testing.T) {
	t.Parallel()
	history, _, _ := newClickHouseHistory(t)
	tasks, err := memory.New(10)
	require.NoError(t, err)
	s := startScheduler(t, tasks, scheduler.WithHistoryStorage(history))
	ctx := t.Context()

	var runs atomic.Int32
	cfg := countingTask("ch-history", &runs)
	require.NoError(t, s.Register(ctx, cfg))
	require.Eventually(t, func() bool {
		page, err := s.HistoryPaginated(ctx, cfg.ID, scheduler.PageRequest{}, `success && taskId == "ch-history"`)
		return err == nil && len(page.Items) == 1
	}, settleWindow, samplingInterval, "the run must be recorded in ClickHouse")

	require.NoError(t, s.Unregister(ctx, cfg.ID))
	for _, err := range history.History(ctx, cfg.ID) {
		require.NoError(t, err)
		require.Fail(t, "Unregister must delete the task's history")
	}
}

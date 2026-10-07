// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/clickhouse"
)

// errQuery stops a query at the connection: the tests inspect the statement,
// not rows.
var errQuery = errors.New("query not served")

// call is one statement the storage sent.
type call struct {
	query string
	args  []any
}

// recordingConn records statements instead of sending them.
type recordingConn struct {
	mu    sync.Mutex
	calls []call
}

func (c *recordingConn) Query(_ context.Context, query string, args ...any) (driver.Rows, error) {
	c.record(query, args)
	return nil, errQuery
}

func (c *recordingConn) Exec(_ context.Context, query string, args ...any) error {
	c.record(query, args)
	return nil
}

func (c *recordingConn) record(query string, args []any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, call{query: query, args: args})
}

func (c *recordingConn) last(tb testing.TB) call {
	tb.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotEmpty(tb, c.calls)
	return c.calls[len(c.calls)-1]
}

func mustNew(tb testing.TB, opts ...clickhouse.Option) (*clickhouse.Storage, *recordingConn) {
	tb.Helper()
	conn := &recordingConn{}
	store, err := clickhouse.New(conn, opts...)
	require.NoError(tb, err)
	return store, conn
}

func TestNew(t *testing.T) {
	t.Parallel()
	_, err := clickhouse.New(nil)
	require.ErrorIs(t, err, clickhouse.ErrNoConn)

	for _, tc := range []struct {
		name string
		opts []clickhouse.Option
		err  error
	}{
		{name: "defaults"},
		{name: "replicated", opts: []clickhouse.Option{
			clickhouse.WithEngine("ReplicatedMergeTree('/clickhouse/tables/{shard}/h', '{replica}')"),
			clickhouse.WithCluster("prod-cluster.eu"),
		}},
		{name: "qualified table", opts: []clickhouse.Option{clickhouse.WithTableName("db.history")}, err: clickhouse.ErrInvalidIdentifier},
		{name: "quoted table", opts: []clickhouse.Option{clickhouse.WithTableName("h`; DROP")}, err: clickhouse.ErrInvalidIdentifier},
		{name: "cluster", opts: []clickhouse.Option{clickhouse.WithCluster("c`")}, err: clickhouse.ErrInvalidIdentifier},
		{name: "engine", opts: []clickhouse.Option{clickhouse.WithEngine("Log")}, err: clickhouse.ErrInvalidEngine},
		{name: "engine expression", opts: []clickhouse.Option{clickhouse.WithEngine("MergeTree(now())")}, err: clickhouse.ErrInvalidEngine},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := clickhouse.New(&recordingConn{}, tc.opts...)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSchemaDDL(t *testing.T) {
	t.Parallel()
	ddl, err := clickhouse.SchemaDDL("h", "MergeTree", "", 7*24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, "CREATE TABLE IF NOT EXISTS `h` (\n"+
		"    `id` String,\n"+
		"    `task_id` String,\n"+
		"    `run_id` String,\n"+
		"    `error` String,\n"+
		"    `started_at` Int64,\n"+
		"    `ended_at` Int64,\n"+
		"    `duration_ms` Int64,\n"+
		"    `success` Bool\n"+
		")\nENGINE = MergeTree"+
		"\nPARTITION BY toYYYYMM(toDateTime(`ended_at`))"+
		"\nORDER BY (`task_id`, `started_at`, `id`)"+
		"\nTTL toDateTime(`ended_at`) + INTERVAL 604800 SECOND", ddl)

	ddl, err = clickhouse.SchemaDDL("h", "MergeTree", "c", 0)
	require.NoError(t, err)
	require.Contains(t, ddl, "CREATE TABLE IF NOT EXISTS `h` ON CLUSTER `c` (")
	require.NotContains(t, ddl, "TTL", "a non-positive ttl keeps history indefinitely")

	ddl, err = clickhouse.SchemaDDL("h", "MergeTree", "", time.Millisecond)
	require.NoError(t, err)
	require.Contains(t, ddl, "INTERVAL 1 SECOND", "a sub-second ttl must not expire rows on insert")

	_, err = clickhouse.SchemaDDL("h", "Memory", "", time.Hour)
	require.ErrorIs(t, err, clickhouse.ErrInvalidEngine)
}

func TestStorage_EnsureSchema(t *testing.T) {
	t.Parallel()
	store, conn := mustNew(t, clickhouse.WithTableName("hist"), clickhouse.WithTTL(time.Hour))
	require.NoError(t, store.EnsureSchema(t.Context()))
	want, err := clickhouse.SchemaDDL("hist", clickhouse.DefaultEngine, "", time.Hour)
	require.NoError(t, err)
	require.Equal(t, want, conn.last(t).query)
}

func TestStorage_EnsureSchemaWithoutTTL(t *testing.T) {
	t.Parallel()
	for _, ttl := range []time.Duration{0, -time.Hour} {
		store, conn := mustNew(t, clickhouse.WithTTL(ttl))
		require.NoError(t, store.EnsureSchema(t.Context()))
		require.NotContains(t, conn.last(t).query, "TTL", "WithTTL(%v) must keep history indefinitely", ttl)
	}
}

func TestStorage_Writes(t *testing.T) {
	t.Parallel()
	store, conn := mustNew(t, clickhouse.WithCluster("c"))
	ctx := t.Context()

	h := &scheduler.TaskHistory{ID: "h", TaskID: "t", RunID: "r", Error: "e", StartedAt: 1, EndedAt: 2, DurationMs: 3, Success: true}
	require.NoError(t, store.AddHistory(ctx, h))
	got := conn.last(t)
	require.Equal(t, "INSERT INTO `scheduler_history` "+
		"(`id`, `task_id`, `run_id`, `error`, `started_at`, `ended_at`, `duration_ms`, `success`) "+
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?)", got.query)
	require.Equal(t, []any{"h", "t", "r", "e", int64(1), int64(2), int64(3), true}, got.args)

	require.NoError(t, store.DeleteHistory(ctx, "t"))
	got = conn.last(t)
	require.Equal(t, "DELETE FROM `scheduler_history` ON CLUSTER `c` WHERE `task_id` = ?", got.query)
	require.Equal(t, []any{"t"}, got.args)

	conn.mu.Lock()
	n := len(conn.calls)
	conn.mu.Unlock()
	require.NoError(t, store.CleanupHistory(ctx, time.Nanosecond))
	conn.mu.Lock()
	defer conn.mu.Unlock()
	require.Len(t, conn.calls, n, "CleanupHistory must leave retention to the table TTL")
}

func TestStorage_HistoryPaginated(t *testing.T) {
	t.Parallel()
	const head = "SELECT `id`, `task_id`, `run_id`, `error`, `started_at`, `ended_at`, `duration_ms`, `success` " +
		"FROM `scheduler_history` WHERE `task_id` = ?"
	const tail = " ORDER BY `started_at` DESC, `id` DESC LIMIT ?"
	for _, tc := range []struct {
		name     string
		pg       scheduler.HistoryPagination
		expr     string
		where    string
		wantArgs []any
	}{
		{
			name:     "first page",
			pg:       scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 10}},
			wantArgs: []any{"t", int64(11)},
		},
		{
			name:     "cursor",
			pg:       scheduler.HistoryPagination{Pagination: scheduler.Pagination{AfterID: "h9", Limit: 2}, AfterStartedAt: 50},
			where:    " AND (`started_at` < ? OR (`started_at` = ? AND `id` < ?))",
			wantArgs: []any{"t", int64(50), int64(50), "h9", int64(3)},
		},
		{
			name:     "filter",
			pg:       scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 1}},
			expr:     `taskId == "x" && durationMs > 5`,
			where:    " AND ((`task_id` = ?) AND (`duration_ms` > ?))",
			wantArgs: []any{"t", "x", int64(5), int64(2)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, conn := mustNew(t)
			var f scheduler.Node
			if tc.expr != "" {
				f = testhelpers.MustParseFilter(t, tc.expr)
			}
			_, err := store.HistoryPaginated(t.Context(), "t", tc.pg, f)
			require.ErrorIs(t, err, errQuery)
			got := conn.last(t)
			require.Equal(t, head+tc.where+tail, got.query)
			require.Equal(t, tc.wantArgs, got.args)
		})
	}

	store, conn := mustNew(t)
	_, err := store.HistoryPaginated(t.Context(), "t", scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 1}},
		testhelpers.MustParseFilter(t, `secret == 1`))
	require.ErrorIs(t, err, filter.ErrFieldNotAllowed, "a field outside HistoryFilterFields must be rejected")
	conn.mu.Lock()
	defer conn.mu.Unlock()
	require.Empty(t, conn.calls, "the query must not be sent")
}

func TestStorage_History(t *testing.T) {
	t.Parallel()
	store, conn := mustNew(t)
	for _, err := range store.History(t.Context(), "t") {
		require.ErrorIs(t, err, errQuery)
	}
	got := conn.last(t)
	require.Equal(t, "SELECT `id`, `task_id`, `run_id`, `error`, `started_at`, `ended_at`, `duration_ms`, `success` "+
		"FROM `scheduler_history` WHERE `task_id` = ? ORDER BY `started_at` DESC, `id` DESC", got.query)
	require.Equal(t, []any{"t"}, got.args)
}

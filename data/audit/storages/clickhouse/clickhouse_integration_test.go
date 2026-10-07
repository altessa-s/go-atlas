// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"

	chgo "github.com/ClickHouse/clickhouse-go/v2"
	auditclickhouse "github.com/altessa-s/go-atlas/data/audit/storages/clickhouse"
)

// envAddr points the integration tests at a live server, for example
// CLICKHOUSE_ADDR=127.0.0.1:9000. The tests skip when it is unset.
const envAddr = "CLICKHOUSE_ADDR"

// newTestConn dials the server named by CLICKHOUSE_ADDR, skipping the test
// when no server is configured.
func newTestConn(t testing.TB) driver.Conn {
	t.Helper()

	addr := os.Getenv(envAddr)
	if addr == "" {
		t.Skipf("%s is not set", envAddr)
	}

	conn, err := chgo.Open(&chgo.Options{
		Addr: []string{addr},
		Auth: chgo.Auth{
			Database: cmpEnv("CLICKHOUSE_DATABASE", "default"),
			Username: cmpEnv("CLICKHOUSE_USER", "default"),
			Password: os.Getenv("CLICKHOUSE_PASSWORD"),
		},
	})
	require.NoError(t, err)
	require.NoError(t, conn.Ping(t.Context()))

	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

func cmpEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

// newTestStorage creates a storage over a table dedicated to the calling
// test and drops that table afterwards.
func newTestStorage(t testing.TB, opts ...auditclickhouse.Option) *auditclickhouse.Storage {
	t.Helper()

	conn := newTestConn(t)
	table := tableName(t)

	require.NoError(t, conn.Exec(t.Context(), fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)))
	t.Cleanup(func() {
		// t.Context is canceled before cleanups run; the drop needs a live context.
		_ = conn.Exec(context.WithoutCancel(t.Context()), fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table))
	})

	opts = append([]auditclickhouse.Option{
		auditclickhouse.WithTableName(table),
		auditclickhouse.WithAutoCreateTable(),
		auditclickhouse.WithTimeRangeMode(auditclickhouse.TimeRangeModeDisabled),
	}, opts...)

	s, err := auditclickhouse.New(conn, opts...)
	require.NoError(t, err)

	return s
}

func tableName(t testing.TB) string {
	t.Helper()

	name := strings.ToLower(t.Name())
	name = strings.NewReplacer("/", "_", "-", "_").Replace(name)

	return "audit_it_" + name
}

// testEvent returns an event whose timestamp is already truncated to the
// millisecond precision DateTime64(3) stores.
func testEvent(id string, ts time.Time) *audit.Event {
	return &audit.Event{
		ID:        id,
		Type:      audit.EventTypeDataChange,
		Action:    audit.ActionUpdate,
		Timestamp: ts.UTC().Truncate(time.Millisecond),
		Duration:  1500 * time.Millisecond,
		Metadata:  map[string]any{"region": "eu-central-1"},
		Actor: audit.Actor{
			Type:  audit.ActorTypeUser,
			ID:    "user-42",
			Name:  "Ada",
			Roles: []string{"admin", "auditor"},
		},
		Resource: audit.Resource{
			Type: "invoice",
			ID:   "inv-7",
			Changes: &audit.ResourceChanges{
				Before: map[string]any{"total": 100.0},
				After:  map[string]any{"total": 120.0},
				Fields: []string{"total"},
			},
		},
		Result:  audit.Result{Status: audit.ResultStatusSuccess, Code: 200},
		Context: audit.EventContext{RequestID: "req-1", TraceID: "trace-1"},
		Service: audit.ServiceInfo{Name: "billing", Version: "1.2.3"},
	}
}

// cursorOf returns the position after e, as FetchPage would decode it from
// the page token.
func cursorOf(e *audit.Event) *audit.Cursor {
	c := audit.CursorOf(e)
	return &c
}

func collect(t testing.TB, seq func(func(*audit.Event, error) bool)) []*audit.Event {
	t.Helper()

	var events []*audit.Event
	for event, err := range seq {
		require.NoError(t, err)
		events = append(events, event)
	}

	return events
}

func TestStorageRoundTrip(t *testing.T) {
	s := newTestStorage(t)

	now := time.Now().UTC()
	want := testEvent("evt-1", now)

	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{want}))

	count, err := s.Count(t.Context(), &audit.Query{ActorID: "user-42"})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	got := collect(t, s.Query(t.Context(), &audit.Query{ActorID: "user-42"}))
	require.Len(t, got, 1)
	auditclickhouse.RequireEventEqual(t, want, got[0])
}

func TestStorageStoreSingleEvent(t *testing.T) {
	s := newTestStorage(t)

	require.NoError(t, s.Store(t.Context(), testEvent("evt-async", time.Now())))

	require.Eventually(t, func() bool {
		count, err := s.Count(t.Context(), nil)

		return err == nil && count == 1
	}, 10*time.Second, 200*time.Millisecond, "asynchronous insert never landed")
}

func TestStorageQueryPaging(t *testing.T) {
	s := newTestStorage(t)

	base := time.Now().UTC().Truncate(time.Millisecond)
	events := make([]*audit.Event, 5)
	for i := range events {
		events[i] = testEvent(fmt.Sprintf("evt-%d", i), base.Add(time.Duration(i)*time.Second))
	}
	require.NoError(t, s.StoreBatch(t.Context(), events))

	t.Run("second page resumes past the cursor", func(t *testing.T) {
		first := collect(t, s.Query(t.Context(), &audit.Query{
			SortOrder: audit.SortOrderAsc,
			Limit:     2,
		}))
		require.Len(t, first, 2)
		require.Equal(t, "evt-0", first[0].ID)
		require.Equal(t, "evt-1", first[1].ID)

		page := &audit.Query{SortOrder: audit.SortOrderAsc, Limit: 2}
		page.Cursor = cursorOf(first[len(first)-1])

		second := collect(t, s.Query(t.Context(), page))
		require.Len(t, second, 2)
		require.Equal(t, "evt-2", second[0].ID)
		require.Equal(t, "evt-3", second[1].ID)
	})

	// Descending is the default direction, and the cursor must follow it:
	// "after" means older here, not newer.
	t.Run("cursor follows the sort direction", func(t *testing.T) {
		first := collect(t, s.Query(t.Context(), &audit.Query{Limit: 1}))
		require.Len(t, first, 1)
		require.Equal(t, "evt-4", first[0].ID, "newest first by default")

		page := &audit.Query{Limit: 1}
		page.Cursor = cursorOf(first[0])

		next := collect(t, s.Query(t.Context(), page))
		require.Len(t, next, 1)
		require.Equal(t, "evt-3", next[0].ID)
	})

	t.Run("cursor past the end yields nothing", func(t *testing.T) {
		last := collect(t, s.Query(t.Context(), &audit.Query{
			SortOrder: audit.SortOrderAsc,
			Limit:     5,
		}))
		require.Len(t, last, 5)

		page := &audit.Query{SortOrder: audit.SortOrderAsc}
		page.Cursor = cursorOf(last[len(last)-1])

		got := collect(t, s.Query(t.Context(), page))
		require.Empty(t, got)
	})
}

// Events written in one batch share a timestamp. Without a tiebreaker in
// ORDER BY their relative order is undefined between queries, and paging
// would skip or repeat rows; the id makes the order total.
func TestStorageQueryPagingIsStableOnEqualTimestamps(t *testing.T) {
	s := newTestStorage(t)

	const total = 6

	ts := time.Now().UTC().Truncate(time.Millisecond)
	events := make([]*audit.Event, total)
	for i := range events {
		events[i] = testEvent(fmt.Sprintf("evt-%d", i), ts)
	}
	require.NoError(t, s.StoreBatch(t.Context(), events))

	seen := make(map[string]int, total)

	query := &audit.Query{Limit: 2}
	for page := 0; page < total/2; page++ {
		got := collect(t, s.Query(t.Context(), query))
		require.Len(t, got, 2, "page %d", page)

		for _, event := range got {
			seen[event.ID]++
		}

		query.Cursor = cursorOf(got[len(got)-1])
	}

	require.Len(t, seen, total, "pages must cover every event exactly once")
	for id, count := range seen {
		require.Equal(t, 1, count, "event %s returned by more than one page", id)
	}
}

func TestStorageQueryTimeRange(t *testing.T) {
	s := newTestStorage(t)

	base := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{
		testEvent("old", base.Add(-2*time.Hour)),
		testEvent("recent", base),
	}))

	start := base.Add(-time.Hour)
	got := collect(t, s.Query(t.Context(), &audit.Query{StartTime: &start}))

	require.Len(t, got, 1)
	require.Equal(t, "recent", got[0].ID)
}

func TestStorageTTLIsAccepted(t *testing.T) {
	s := newTestStorage(t, auditclickhouse.WithTTL(30*24*time.Hour))

	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{testEvent("evt-ttl", time.Now())}))

	count, err := s.Count(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

// CheckSchema reads system.columns and system.data_skipping_indices, so its
// queries are only ever really verified against a server.
func TestStorageCheckSchemaAcceptsFreshTable(t *testing.T) {
	s := newTestStorage(t, auditclickhouse.WithSchemaCheck(auditclickhouse.SchemaCheckModeEnforce))

	require.NoError(t, s.CheckSchema(t.Context()))
}

// A table created before an index was added to this package keeps none, and
// CREATE TABLE IF NOT EXISTS will not add it. Dropping an index reproduces
// exactly that drift.
func TestStorageCheckSchemaDetectsDrift(t *testing.T) {
	conn := newTestConn(t)
	table := tableName(t)

	require.NoError(t, conn.Exec(t.Context(), fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)))
	t.Cleanup(func() {
		_ = conn.Exec(context.WithoutCancel(t.Context()), fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table))
	})

	s, err := auditclickhouse.New(conn,
		auditclickhouse.WithTableName(table),
		auditclickhouse.WithAutoCreateTable(),
		auditclickhouse.WithSchemaCheck(auditclickhouse.SchemaCheckModeEnforce),
	)
	require.NoError(t, err)
	require.NoError(t, s.CheckSchema(t.Context()), "a table this package just created must match")

	require.NoError(t, conn.Exec(t.Context(), fmt.Sprintf("ALTER TABLE `%s` DROP INDEX idx_trace_id", table)))

	err = s.CheckSchema(t.Context())
	require.ErrorIs(t, err, auditclickhouse.ErrSchemaMismatch)
	require.ErrorContains(t, err, "idx_trace_id")
}

// The additive migration must be accepted by a real server: ALTER ... ADD
// COLUMN / ADD INDEX with IF NOT EXISTS is the whole contract, and only
// ClickHouse can confirm it parses and applies.
func TestStorageMigrateSchemaRepairsDrift(t *testing.T) {
	conn := newTestConn(t)
	table := tableName(t)

	require.NoError(t, conn.Exec(t.Context(), fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)))
	t.Cleanup(func() {
		_ = conn.Exec(context.WithoutCancel(t.Context()), fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table))
	})

	s, err := auditclickhouse.New(conn,
		auditclickhouse.WithTableName(table),
		auditclickhouse.WithAutoCreateTable(),
		auditclickhouse.WithSchemaCheck(auditclickhouse.SchemaCheckModeEnforce),
		auditclickhouse.WithSchemaMigration(auditclickhouse.SchemaMigrationModeAdditive),
	)
	require.NoError(t, err)

	// Reproduce a table created before the index existed.
	require.NoError(t, conn.Exec(t.Context(), fmt.Sprintf("ALTER TABLE `%s` DROP INDEX idx_trace_id", table)))
	require.Error(t, s.CheckSchema(t.Context()), "the drift must be visible first")

	require.NoError(t, s.MigrateSchema(t.Context()))
	require.NoError(t, s.CheckSchema(t.Context()), "migration must close the drift")

	// Idempotent: IF NOT EXISTS means a second replica starting up is a
	// no-op rather than an error.
	require.NoError(t, s.MigrateSchema(t.Context()))
	require.NoError(t, s.CheckSchema(t.Context()))
}

// Writing and reading must still work after the table was altered underneath.
func TestStorageMigrateSchemaKeepsTableUsable(t *testing.T) {
	conn := newTestConn(t)
	table := tableName(t)

	require.NoError(t, conn.Exec(t.Context(), fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)))
	t.Cleanup(func() {
		_ = conn.Exec(context.WithoutCancel(t.Context()), fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table))
	})

	s, err := auditclickhouse.New(conn,
		auditclickhouse.WithTableName(table),
		auditclickhouse.WithAutoCreateTable(),
		auditclickhouse.WithTimeRangeMode(auditclickhouse.TimeRangeModeDisabled),
		auditclickhouse.WithSchemaMigration(auditclickhouse.SchemaMigrationModeAdditive),
	)
	require.NoError(t, err)

	require.NoError(t, conn.Exec(t.Context(), fmt.Sprintf("ALTER TABLE `%s` DROP INDEX idx_resource_id", table)))
	require.NoError(t, s.MigrateSchema(t.Context()))

	want := testEvent("evt-migrated", time.Now())
	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{want}))

	got := collect(t, s.Query(t.Context(), &audit.Query{ResourceID: "inv-7"}))
	require.Len(t, got, 1)
	auditclickhouse.RequireEventEqual(t, want, got[0])
}

// MATERIALIZE INDEX is a mutation; the server must at least accept it.
func TestStorageMaterializeIndexes(t *testing.T) {
	s := newTestStorage(t)

	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{testEvent("evt-1", time.Now())}))
	require.NoError(t, s.MaterializeIndexes(t.Context()))
}

// The unmaterialized heuristic rests on one assumption — that an index added
// over populated parts reports zero compressed bytes — and only a server can
// confirm it.
func TestStorageDetectsUnmaterializedIndex(t *testing.T) {
	conn := newTestConn(t)
	table := tableName(t)

	require.NoError(t, conn.Exec(t.Context(), fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)))
	t.Cleanup(func() {
		_ = conn.Exec(context.WithoutCancel(t.Context()), fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table))
	})

	var logs bytes.Buffer

	s, err := auditclickhouse.New(conn,
		auditclickhouse.WithTableName(table),
		auditclickhouse.WithAutoCreateTable(),
		auditclickhouse.WithSchemaMigration(auditclickhouse.SchemaMigrationModeAdditive),
		auditclickhouse.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))),
	)
	require.NoError(t, err)

	// Populate first, then drop and re-add the index: the parts on disk now
	// predate it, which is exactly the state the heuristic looks for.
	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{testEvent("evt-1", time.Now())}))
	require.NoError(t, conn.Exec(t.Context(), fmt.Sprintf("ALTER TABLE `%s` DROP INDEX idx_trace_id", table)))
	require.NoError(t, s.MigrateSchema(t.Context()))

	require.NoError(t, s.CheckSchema(t.Context()), "an unmaterialized index must not fail the check")
	require.Contains(t, logs.String(), "hold no data")
	require.Contains(t, logs.String(), "idx_trace_id")

	// After materializing, the warning must go away.
	require.NoError(t, s.MaterializeIndexes(t.Context()))

	require.Eventually(t, func() bool {
		var fresh bytes.Buffer

		checked, newErr := auditclickhouse.New(conn,
			auditclickhouse.WithTableName(table),
			auditclickhouse.WithLogger(slog.New(slog.NewTextHandler(&fresh, nil))),
		)
		require.NoError(t, newErr)
		require.NoError(t, checked.CheckSchema(t.Context()))

		return !strings.Contains(fresh.String(), "hold no data")
	}, 10*time.Second, 200*time.Millisecond, "materialization never became visible")
}

func TestStorageCheckSchemaMissingTable(t *testing.T) {
	conn := newTestConn(t)

	s, err := auditclickhouse.New(conn,
		auditclickhouse.WithTableName("audit_it_absent_table"),
		auditclickhouse.WithSchemaCheck(auditclickhouse.SchemaCheckModeEnforce),
	)
	require.NoError(t, err)

	err = s.CheckSchema(t.Context())
	require.ErrorIs(t, err, auditclickhouse.ErrSchemaMismatch)
	require.ErrorContains(t, err, "does not exist")
}

// Warn is the default for a reason: drift is reported, not fatal.
func TestStorageCheckSchemaWarnDoesNotFail(t *testing.T) {
	conn := newTestConn(t)

	s, err := auditclickhouse.New(conn,
		auditclickhouse.WithTableName("audit_it_absent_table"),
		auditclickhouse.WithSchemaCheck(auditclickhouse.SchemaCheckModeWarn),
	)
	require.NoError(t, err)

	require.NoError(t, s.CheckSchema(t.Context()))
}

// At-least-once delivery can replay an event; ReplacingMergeTree collapses
// the copies by the sorting key, but only for readers that ask for FINAL.
func TestStorageDeduplicatesReplayedEvents(t *testing.T) {
	s := newTestStorage(t, auditclickhouse.WithFinal())

	event := testEvent("evt-replay", time.Now())
	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{event}))
	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{event}))

	count, err := s.Count(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

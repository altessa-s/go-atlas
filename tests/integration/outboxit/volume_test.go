//go:build volume

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

const (
	volumeBatchSize = 1000
	volumeWorkers   = 4
	volumeLifetime  = 24 * time.Hour
)

type volumeGroup struct {
	name   string
	n      int
	status outbox.Status
}

type volumeSeed struct {
	n      int
	now    time.Time
	oldest time.Time
	groups []volumeGroup
	total  int
}

func newVolumeSeed(t testing.TB) volumeSeed {
	t.Helper()
	n := livebackend.VolumeRows(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	s := volumeSeed{n: n, now: now, groups: []volumeGroup{
		{"pending", n, outbox.StatusPending},
		{"old", n / 10, outbox.StatusSent},
		{"recent", n / 20, outbox.StatusSent},
		// Failed rows have expired, so dispatch can expire them without
		// delivering anything outside the eligible pending cohort.
		{"failed", n / 40, outbox.StatusFailed},
		{"dead", n / 40, outbox.StatusMaxAttemptReached},
		{"rejected", n / 40, outbox.StatusRejected},
		{"expired", n / 20, outbox.StatusPending},
	}}
	for _, g := range s.groups {
		s.total += g.n
	}
	// Millisecond spacing survives BSON Date precision. Even the newest row
	// predates seeding by a day, independently of ATLAS_VOLUME_ROWS.
	s.oldest = now.Add(-volumeLifetime - time.Duration(s.total)*time.Millisecond)
	return s
}

func volumeID(t testing.TB, group string, i int) string {
	t.Helper()
	return fmt.Sprintf("%s-%012d", group, i)
}

func volumeEvent(t testing.TB, s volumeSeed, g volumeGroup, offset, i int) outbox.Event {
	t.Helper()
	ev := outbox.Event{
		Id: volumeID(t, g.name, i), Key: fmt.Sprintf("orders.%d", i%8),
		Payload: []byte(`{"volume":true}`), Status: g.status,
		CreatedAt: s.oldest.Add(time.Duration(offset+i) * time.Millisecond),
	}
	switch g.name {
	case "old":
		ev.PublishedAt = s.now.Add(-7 * volumeLifetime)
	case "recent":
		ev.PublishedAt = s.now
	case "expired", "failed":
		ev.ExpiresAt = s.now.Add(-time.Hour)
	}
	if g.status == outbox.StatusFailed || g.status == outbox.StatusMaxAttemptReached || g.status == outbox.StatusRejected {
		ev.Attempts = 3
		ev.LastError = new("seeded failure")
		ev.LastAttemptOn = s.now.Add(-2 * time.Hour)
	}
	return ev
}

type volumeBackend struct {
	store outbox.Storage
	// An empty group counts all rows; otherwise count the deterministic ID
	// range, independent of status changes made by a cycle.
	count func(testing.TB, string) int64
}

func newVolumeOutbox(t testing.TB, store outbox.Storage, handler outbox.Handler) *outbox.Outbox {
	t.Helper()
	return outbox.New(store, handler,
		outbox.WithContext(t.Context()), outbox.WithEventsBatchSize(volumeBatchSize),
		outbox.WithFetchTimeout(time.Minute), outbox.WithHandleTimeout(time.Minute),
		outbox.WithUpdateTimeout(time.Minute), outbox.WithMaxLockTime(5*time.Minute),
		outbox.WithPublishedEventsLifetime(volumeLifetime))
}

// volumeObservedStore checks the public fetch contract before concurrent
// handlers reorder execution. One wrapper belongs to each dispatcher. It also
// surfaces UpdateEvents errors, which the dispatch cycle only logs.
type volumeObservedStore struct {
	outbox.Storage
	fetched int
	err     error
}

func (s *volumeObservedStore) FetchUnprocessedEvents(ctx context.Context, size uint32) ([]outbox.Event, error) {
	events, err := s.Storage.FetchUnprocessedEvents(ctx, size)
	s.fetched = len(events)
	if err != nil {
		return nil, err
	}
	for i := 1; i < len(events); i++ {
		if !events[i-1].CreatedAt.Before(events[i].CreatedAt) {
			return nil, fmt.Errorf("claim batch is not oldest-first: %s (%s) before %s (%s)",
				events[i-1].Id, events[i-1].CreatedAt, events[i].Id, events[i].CreatedAt)
		}
	}
	return events, nil
}

func (s *volumeObservedStore) UpdateEvents(ctx context.Context, events ...outbox.Event) error {
	s.err = s.Storage.UpdateEvents(ctx, events...)
	return s.err
}

func volumeDispatch(t testing.TB, b volumeBackend, seed volumeSeed) {
	t.Helper()
	var mu sync.Mutex
	counts := make(map[string]uint32, seed.n)
	handler := func(_ context.Context, ev outbox.Event) error {
		mu.Lock()
		counts[ev.Id]++
		mu.Unlock()
		return nil
	}
	workers := make([]*outbox.Outbox, volumeWorkers)
	observed := make([]*volumeObservedStore, volumeWorkers)
	for i := range workers {
		observed[i] = &volumeObservedStore{Storage: b.store}
		workers[i] = newVolumeOutbox(t, observed[i], handler)
	}
	livebackend.Measure(t, "expire before dispatch", seed.n/20+seed.n/40, func() {
		require.NoError(t, workers[0].RunExpireCycle(t.Context()))
		n, err := b.store.ExpireEvents(t.Context())
		require.NoError(t, err)
		require.Zero(t, n, "the manual expire cycle must exhaust expired rows")
		stats, err := b.store.Stats(t.Context())
		require.NoError(t, err)
		require.EqualValues(t, seed.n, stats.Pending)
	})
	livebackend.Measure(t, "dispatch with four instances", seed.n, func() {
		// Each instance drains until its own fetch is empty. SKIP LOCKED may
		// make one finish early while another still owns the last batch.
		err := concurrency.Process(t.Context(), []int{0, 1, 2, 3}, func(ctx context.Context, i int) error {
			for range seed.n/volumeBatchSize + 5 {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := workers[i].RunDispatchCycle(ctx); err != nil {
					return err
				}
				if observed[i].err != nil {
					return observed[i].err
				}
				if observed[i].fetched == 0 {
					return nil
				}
			}
			return fmt.Errorf("dispatcher %d did not drain its backlog", i)
		}, concurrency.WithConcurrency[int](volumeWorkers), concurrency.WithStopOnError[int]())
		require.NoError(t, err)
	})
	livebackend.Measure(t, "verify deliveries and final stats", seed.n, func() {
		// Process has joined every worker; the handler map is now quiescent.
		require.Len(t, counts, seed.n, "only the eligible pending cohort may be delivered")
		for i := range seed.n {
			id := volumeID(t, "pending", i)
			require.EqualValues(t, 1, counts[id], "delivery count for %s", id)
		}
		stats, err := b.store.Stats(t.Context())
		require.NoError(t, err)
		require.Zero(t, stats.Pending)
		require.Zero(t, stats.InProgress)
		require.Zero(t, stats.OldestPendingAge)
		require.EqualValues(t, 2*(seed.n/40), stats.DeadLettered)
		require.EqualValues(t, seed.total, b.count(t, ""), "dispatch must not delete rows")
	})
}

func volumeCleanup(t testing.TB, b volumeBackend, seed volumeSeed) {
	t.Helper()
	ob := newVolumeOutbox(t, b.store, func(context.Context, outbox.Event) error { return nil })
	livebackend.Measure(t, "counts before retention", seed.total, func() {
		require.EqualValues(t, seed.total, b.count(t, ""))
		for _, g := range seed.groups {
			require.EqualValues(t, g.n, b.count(t, g.name), "%s before cleanup", g.name)
		}
	})
	livebackend.Measure(t, "cleanup old published events", seed.n/10, func() {
		// The cycle returns only an error. Observe deletion counts directly;
		// repeat until a cycle deletes nothing, including batch-limited stores.
		before := b.count(t, "")
		var removed int64
		for {
			require.NoError(t, t.Context().Err())
			require.NoError(t, ob.RunCleanupCycle(t.Context()))
			after := b.count(t, "")
			require.LessOrEqual(t, after, before)
			removed += before - after
			if after == before {
				break
			}
			before = after
		}
		require.EqualValues(t, seed.n/10, removed)
	})
	livebackend.Measure(t, "verify retained cohorts", seed.total-seed.n/10, func() {
		require.EqualValues(t, seed.total-seed.n/10, b.count(t, ""))
		for _, g := range seed.groups {
			want := g.n
			if g.name == "old" {
				want = 0
			}
			require.EqualValues(t, want, b.count(t, g.name), "%s after cleanup", g.name)
		}
		volumeStats(t, b, seed)
	})
}

func volumeStats(t testing.TB, b volumeBackend, seed volumeSeed) {
	t.Helper()
	livebackend.Measure(t, "backlog stats", seed.total, func() {
		stats, err := b.store.Stats(t.Context())
		require.NoError(t, err)
		require.EqualValues(t, seed.n+seed.n/20+seed.n/40, stats.Pending, "pending includes failed and past-expiry rows")
		require.Zero(t, stats.InProgress)
		require.EqualValues(t, 2*(seed.n/40), stats.DeadLettered)
		require.Positive(t, stats.OldestPendingAge)
		// The age uses the server clock. Allow a minute for host/server skew,
		// while still proving the oldest seeded row supplies the timestamp.
		require.InDelta(t, time.Since(seed.oldest).Seconds(), stats.OldestPendingAge.Seconds(), time.Minute.Seconds())
	})
}

// Volume scenarios run serially: their large backlogs share database resources.
// Each test has exactly one subtest per backend, including MongoDB below.
func TestVolume_Dispatch(t *testing.T) {
	for _, target := range volumeTargets(t) {
		t.Run(target.name, func(t *testing.T) {
			seed := newVolumeSeed(t)
			volumeDispatch(t, target.open(t, seed), seed)
		})
	}
}

func TestVolume_Cleanup(t *testing.T) {
	for _, target := range volumeTargets(t) {
		t.Run(target.name, func(t *testing.T) {
			seed := newVolumeSeed(t)
			volumeCleanup(t, target.open(t, seed), seed)
		})
	}
}

func TestVolume_Stats(t *testing.T) {
	for _, target := range volumeTargets(t) {
		t.Run(target.name, func(t *testing.T) {
			seed := newVolumeSeed(t)
			volumeStats(t, target.open(t, seed), seed)
		})
	}
}

type volumeTarget struct {
	name string
	open func(testing.TB, volumeSeed) volumeBackend
}

func volumeTargets(t testing.TB) []volumeTarget {
	t.Helper()
	targets := make([]volumeTarget, 0, 4)
	for _, target := range livebackend.SQLTargets() {
		targets = append(targets, volumeTarget{target.Name, func(t testing.TB, seed volumeSeed) volumeBackend {
			t.Helper()
			_, b := newVolumeSQL(t, target, seed)
			return b
		}})
	}
	targets = append(targets, volumeTarget{"mongo", func(t testing.TB, seed volumeSeed) volumeBackend {
		t.Helper()
		_, b := newVolumeMongo(t, seed)
		return b
	}})
	return targets
}

func newVolumeSQL(t testing.TB, target livebackend.SQLTarget, seed volumeSeed) (*sqlBackend, volumeBackend) {
	t.Helper()
	db := target.Open(t)
	// Keep the dispatchers' connections pooled: the default two idle
	// connections make concurrent cycles redial, exhausting ephemeral ports.
	db.SetMaxIdleConns(64)
	b := &sqlBackend{db: db, dialect: outboxsql.Dialect(target.Dialect), events: livebackend.SQLTable(t, db, "obvol")}
	store, err := outboxsql.New(db, b.dialect, outboxsql.WithTableName(b.events))
	require.NoError(t, err)
	require.NoError(t, store.EnsureSchema(t.Context()))
	livebackend.Measure(t, "seed SQL outbox", seed.total, func() {
		offset := 0
		for _, g := range seed.groups {
			livebackend.SQLBulkInsert(t, db, target.Dialect, b.events,
				[]string{"id", "topic", "payload", "status", "created_at", "attempts", "error", "last_attempt_on", "published_at", "expires_at"},
				g.n, func(i int) []any {
					ev := volumeEvent(t, seed, g, offset, i)
					var lastError any
					if ev.LastError != nil {
						lastError = []byte(*ev.LastError)
					}
					return []any{ev.Id, []byte(ev.Key), ev.Payload, string(ev.Status), volumeSQLTime(t, b, ev.CreatedAt),
						int64(ev.Attempts), lastError, volumeSQLTime(t, b, ev.LastAttemptOn),
						volumeSQLTime(t, b, ev.PublishedAt), volumeSQLTime(t, b, ev.ExpiresAt)}
				})
			offset += g.n
		}
	})
	return b, volumeBackend{store: store, count: func(t testing.TB, group string) int64 {
		t.Helper()
		query := "SELECT COUNT(*) FROM " + b.events
		var args []any
		if group != "" {
			query += " WHERE id >= ? AND id < ?"
			args = []any{group + "-", group + "."}
		}
		var n int64
		require.NoError(t, db.QueryRowContext(t.Context(), b.bind(query), args...).Scan(&n))
		return n
	}}
}

func volumeSQLTime(t testing.TB, b *sqlBackend, v time.Time) any {
	t.Helper()
	if v.IsZero() {
		return nil
	}
	if b.dialect == outboxsql.DialectPostgres {
		return v.UTC()
	}
	return v.UTC().Format(mysqlTimeLayout)
}

type volumeQuery struct{ name, text string }

func TestVolume_Explain(t *testing.T) {
	// Serial for the same database resource budget as the correctness tests.
	for _, target := range livebackend.SQLTargets() {
		t.Run(target.Name, func(t *testing.T) {
			seed := newVolumeSeed(t)
			b, _ := newVolumeSQL(t, target, seed)
			livebackend.Measure(t, "analyze SQL outbox", seed.total, func() {
				livebackend.Analyze(t, b.db, string(b.dialect), b.events)
			})
			// One subtest per query, so a full scan does not hide later hot queries.
			for _, q := range volumeSQLQueries(t, b) {
				t.Run(q.name, func(t *testing.T) {
					if b.dialect == outboxsql.DialectMySQL {
						access, key := livebackend.ExplainMySQL(t, b.db, b.events, volumeMySQLKeys(b.events, q.name), q.text)
						t.Logf("%s: access=%s key=%s", q.name, access, key)
						return
					}
					switch q.name {
					case "delete_processed":
						// The batch select (delete_batch) must use published_idx at
						// any size. Joining its ids back, the planner hashes a
						// sequential scan of a small table; only a large one must
						// take the primary key.
						if livebackend.VolumeRows(t) < volumeOuterScanRows {
							t.Logf("%s: plan below %d rows, not asserted:\n%s", q.name, volumeOuterScanRows, volumePostgresPlan(t, b, q.text))
							return
						}
					case "delete_batch":
						indexes := livebackend.ExplainPostgres(t, b.db, b.events, q.text)
						require.Contains(t, indexes, b.events+"_published_idx", "%s: indexes %v", q.name, indexes)
						return
					case "stats":
						// KNOWN FULL SCAN: the status filter matches nearly the
						// whole backlog, so a sequential scan is the right plan;
						// the count is O(backlog) by design, not for a missing index.
						t.Logf("KNOWN FULL SCAN %s:\n%s", q.name, volumePostgresPlan(t, b, q.text))
						return
					}
					t.Logf("%s: indexes %v", q.name, livebackend.ExplainPostgres(t, b.db, b.events, q.text))
				})
			}
		})
	}
	t.Run("mongo", func(t *testing.T) { volumeMongoExplain(t) })
}

// volumeOuterScanRows is the seed size from which the outer DELETE of the
// PostgreSQL retention purge must read the table through its primary key.
const volumeOuterScanRows = 100_000

// volumePostgresPlan returns the JSON plan of query, for logging a plan the
// test does not assert.
func volumePostgresPlan(t testing.TB, b *sqlBackend, query string) string {
	t.Helper()
	var plan string
	require.NoError(t, b.db.QueryRowContext(t.Context(), "EXPLAIN (FORMAT JSON) "+query).Scan(&plan))
	return plan
}

// volumeMySQLKeys lists the indexes each hot query may read the events table
// through on MySQL and MariaDB: the ones reliability/outbox/storages/sqldb/schema.go
// declares for it. Status leads every index, so a query whose status set is
// small may also be served by a range over another status-led index — the
// optimizer picks expires for the failed fetch and fetch for the stuck sweep
// at 20k rows; both read only the rows of that status.
func volumeMySQLKeys(table, query string) []string {
	idx := func(suffix string) string { return table + "_" + suffix + "_idx" }
	switch query {
	case "fetch_pending":
		return []string{idx("fetch")}
	case "fetch_failed":
		return []string{idx("fetch"), idx("expires")}
	case "unlock":
		return []string{"PRIMARY"}
	case "stuck_sweep":
		return []string{idx("locked"), idx("fetch")}
	case "delete_processed":
		return []string{idx("published")}
	case "expire":
		return []string{idx("expires")}
	default:
		return []string{idx("fetch"), idx("locked"), idx("published"), idx("expires")}
	}
}

func volumeSQLQueries(t testing.TB, b *sqlBackend) []volumeQuery {
	t.Helper()
	// SQL shapes copied from reliability/outbox/storages/sqldb/store.go:
	// Store.buildStatements and Store.oldestAge. Concrete policy values match
	// newVolumeOutbox. Keep locking clauses, DML, projections and LIMITs intact.
	table := b.events
	now, cutoff, lockCutoff := "now()", "(now() - make_interval(secs => 86400))", "(now() - make_interval(secs => 300))"
	if b.dialect == outboxsql.DialectMySQL {
		now = "UTC_TIMESTAMP(6)"
		cutoff = "(UTC_TIMESTAMP(6) - INTERVAL 86400000000 MICROSECOND)"
		lockCutoff = "(UTC_TIMESTAMP(6) - INTERVAL 300000000 MICROSECOND)"
	}
	cols := "id, topic, payload, attempts, error, " + b.timeCol("created_at") + ", " + b.timeCol("published_at") + ", " +
		b.timeCol("last_attempt_on") + ", " + b.timeCol("expires_at") + ", seq"
	ready := " AND (expires_at IS NULL OR expires_at > " + now + ") ORDER BY created_at, seq LIMIT 1000 FOR UPDATE SKIP LOCKED"
	stuck := "status = 'in-progress' AND locked_on < " + lockCutoff
	processed := "status IN ('sent', 'skipped', 'expired') AND published_at < " + cutoff
	unlockSet := " SET status = 'pending', locked_on = NULL, lock_token = NULL, next_attempt_at = NULL"
	queries := []volumeQuery{
		{"fetch_pending", "SELECT " + cols + " FROM " + table + " WHERE status = 'pending'" + ready},
		{"fetch_failed", "SELECT " + cols + " FROM " + table + " WHERE status = 'failed' AND (next_attempt_at IS NULL OR next_attempt_at <= " + now + ")" + ready},
		{"unlock", "UPDATE " + table + unlockSet + " WHERE id IN (SELECT id FROM " + table + " WHERE " + stuck + " ORDER BY id FOR UPDATE)"},
		{"delete_processed", "DELETE FROM " + table + " WHERE id IN (SELECT id FROM " + table + " WHERE " + processed + " LIMIT 1000 FOR UPDATE SKIP LOCKED)"},
		{"expire", "UPDATE " + table + " SET status = 'expired', published_at = " + now + ", locked_on = NULL, lock_token = NULL, next_attempt_at = NULL" +
			" WHERE status IN ('pending', 'failed') AND expires_at IS NOT NULL AND expires_at <= " + now},
	}
	if b.dialect == outboxsql.DialectMySQL {
		queries[2].text = "UPDATE " + table + " FORCE INDEX (PRIMARY)" + unlockSet + " WHERE id IN ('pending-000000000000') AND " + stuck
		queries[3].text = "DELETE FROM " + table + " WHERE " + processed + " LIMIT 1000"
		queries = append(queries, volumeQuery{"stuck_sweep", "SELECT id FROM " + table + " WHERE " + stuck + " AND id > '' ORDER BY id LIMIT 1000"})
	} else {
		// The batch select of delete_processed, explained on its own.
		queries = append(queries, volumeQuery{"delete_batch", "SELECT id FROM " + table + " WHERE " + processed + " LIMIT 1000 FOR UPDATE SKIP LOCKED"})
	}
	oldest := "MIN(CASE WHEN status IN ('pending', 'failed') THEN created_at END)"
	age := "COALESCE(CAST(EXTRACT(EPOCH FROM (now() - " + oldest + ")) * 1000 AS BIGINT), 0)"
	if b.dialect == outboxsql.DialectMySQL {
		age = "COALESCE(TIMESTAMPDIFF(MICROSECOND, " + oldest + ", UTC_TIMESTAMP(6)) DIV 1000, 0)"
	}
	// Most seeded rows are backlog; see the KNOWN FULL SCAN note in
	// TestVolume_Explain for PostgreSQL.
	queries = append(queries, volumeQuery{"stats", "SELECT" +
		" COALESCE(SUM(CASE WHEN status IN ('pending', 'failed') THEN 1 ELSE 0 END), 0)," +
		" COALESCE(SUM(CASE WHEN status = 'in-progress' THEN 1 ELSE 0 END), 0)," +
		" COALESCE(SUM(CASE WHEN status IN ('max-attempt-reached', 'rejected') THEN 1 ELSE 0 END), 0), " + age +
		" FROM " + table + " WHERE status IN ('in-progress', 'pending', 'failed', 'max-attempt-reached', 'rejected')"})
	return queries
}

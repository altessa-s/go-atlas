//go:build volume

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueueit_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/reliability/retryqueue"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/sqldb"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/storagetest"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	retrymongo "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/mongo"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

// Volume seed layout: row i belongs to the category i%10 picks.
const (
	volPage         = 1000            // ListDue and ListByState page size
	volWorkers      = 16              // concurrent Resolve calls per page
	volRequeueKeys  = 2500            // expired keys of the chunked Requeue (> 1000, the statement chunk)
	volForeignKeys  = 500             // non-expired keys mixed into that Requeue
	volRetention    = time.Hour       // resolved retention of the purge
	volPurgeBatch   = 5000            // rows per purge statement
	volFlaggedEvery = 50              // every 50th row (a due one) is flagged
	volFlagReason   = "manual review" // reason of the flagged rows
)

// volCategory is the seeded state of a row.
type volCategory int

const (
	volDue         volCategory = iota // pending, due at volT0
	volNotDue                         // pending, due an hour after volT0
	volExpired                        // expired
	volResolvedOld                    // resolved two retentions before volT0: purged
	volResolvedNew                    // resolved at volT0: kept
)

// volT0 is the reference instant of the seed: whole milliseconds, so MongoDB
// round-trips it exactly.
var volT0 = time.Now().UTC().Truncate(time.Second)

func volCategoryOf(i int) volCategory {
	switch i % 10 {
	case 0:
		return volDue
	case 1, 2, 3, 4, 5:
		return volNotDue
	case 6, 7:
		return volExpired
	case 8:
		return volResolvedOld
	default:
		return volResolvedNew
	}
}

func volKey(i int) string { return fmt.Sprintf("k%09d", i) }

// volSince is non-decreasing in i with ties of three, so ListByState's
// (since, key) order is the order of i.
func volSince(i int) time.Time { return volT0.Add(-24*time.Hour + time.Duration(i/3)*time.Millisecond) }

// volNext is non-decreasing in i with ties among due rows, so ListDue's
// (next attempt, key) order of the due rows is the order of i.
func volNext(i int) time.Time {
	if volCategoryOf(i) == volNotDue {
		return volT0.Add(time.Hour + time.Duration(i)*time.Millisecond)
	}
	return volT0.Add(-time.Hour + time.Duration(i/30)*time.Millisecond)
}

func volExpiredAt(i int) time.Time {
	if volCategoryOf(i) == volExpired {
		return volT0.Add(-30 * time.Minute)
	}
	return time.Time{}
}

func volResolvedAt(i int) time.Time {
	switch volCategoryOf(i) {
	case volResolvedOld:
		return volT0.Add(-2 * volRetention)
	case volResolvedNew:
		return volT0
	default:
		return time.Time{}
	}
}

func volFlagged(i int) bool { return i%volFlaggedEvery == 0 }

func volState(i int) retryqueue.State {
	switch volCategoryOf(i) {
	case volDue, volNotDue:
		return retryqueue.StatePending
	case volExpired:
		return retryqueue.StateExpired
	default:
		return retryqueue.StateResolved
	}
}

func volPayload(i int) storagetest.Payload {
	return storagetest.Payload{ValueOn: volT0, Note: fmt.Sprintf("category-%d", volCategoryOf(i))}
}

// volCounts returns how many of n rows fall into each category.
func volCounts(n int) map[volCategory]int {
	out := map[volCategory]int{}
	for i := range n {
		out[volCategoryOf(i)]++
	}
	return out
}

// volKeysOf returns the indexes, in order, of the first limit rows of the
// categories in cats (all of them when limit is negative).
func volKeysOf(n, limit int, cats ...volCategory) []int {
	var out []int
	for i := 0; i < n && (limit < 0 || len(out) < limit); i++ {
		for _, c := range cats {
			if volCategoryOf(i) == c {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

// volIndex parses the row index back out of a key.
func volIndex(tb testing.TB, key string) int {
	tb.Helper()
	var i int
	_, err := fmt.Sscanf(key, "k%09d", &i)
	require.NoError(tb, err, key)
	return i
}

// volSweep pages through ListDue at volT0 like a sweeper, resolving each page
// before listing the next, and asserts every due row is listed exactly once,
// in (next attempt, key) order, with its flag and payload.
func volSweep(t *testing.T, s storagetest.Storage, n int) {
	t.Helper()
	ctx := t.Context()
	counts := volCounts(n)
	seen := make([]bool, n)
	listed, last := 0, -1
	livebackend.Measure(t, "ListDue sweep (list + resolve)", counts[volDue], func() {
		for {
			page, err := s.ListDue(ctx, volT0, volPage)
			require.NoError(t, err)
			if len(page) == 0 {
				return
			}
			for _, it := range page {
				i := volIndex(t, it.Key)
				require.Equal(t, volDue, volCategoryOf(i), "listed a row that is not due: %s", it.Key)
				require.False(t, seen[i], "listed twice: %s", it.Key)
				require.Greater(t, i, last, "out of (next attempt, key) order: %s", it.Key)
				require.Equal(t, volNext(i), it.NextAttemptAt, it.Key)
				require.Equal(t, volFlagged(i), it.Flagged, it.Key)
				if volFlagged(i) {
					require.Equal(t, volFlagReason, it.FlagReason, it.Key)
				}
				require.Equal(t, volPayload(i), it.Payload, it.Key)
				seen[i], last = true, i
			}
			listed += len(page)
			require.NoError(t, concurrency.Process(ctx, page, func(ctx context.Context, it storagetest.Item) error {
				return s.Resolve(ctx, it.Key, it.Generation, volT0)
			}, concurrency.WithConcurrency[storagetest.Item](volWorkers)))
		}
	})
	require.Equal(t, counts[volDue], listed)
}

// volListByState asserts the first page of each state is the expected rows in
// (since, key) order. Run after the sweep: due rows are resolved by then.
func volListByState(t *testing.T, s storagetest.Storage, n int) {
	t.Helper()
	want := map[retryqueue.State][]int{
		retryqueue.StatePending:  volKeysOf(n, volPage, volNotDue),
		retryqueue.StateExpired:  volKeysOf(n, volPage, volExpired),
		retryqueue.StateResolved: volKeysOf(n, volPage, volDue, volResolvedOld, volResolvedNew),
	}
	for state, rows := range want {
		var got []storagetest.Item
		livebackend.Measure(t, "ListByState "+string(state), volPage, func() {
			var err error
			got, err = s.ListByState(t.Context(), state, volPage)
			require.NoError(t, err)
		})
		require.Len(t, got, len(rows), state)
		for k, it := range got {
			require.Equal(t, volKey(rows[k]), it.Key, "%s position %d", state, k)
			require.Equal(t, state, it.State, it.Key)
			require.Equal(t, volSince(rows[k]), it.Since, it.Key)
		}
	}
}

// volRequeue requeues a key list longer than one statement chunk, mixed with
// keys that are not expired, then every remaining expired row, and asserts the
// exact counts.
func volRequeue(t *testing.T, s storagetest.Storage, n int) {
	t.Helper()
	ctx := t.Context()
	counts := volCounts(n)
	expired := volKeysOf(n, volRequeueKeys, volExpired)
	require.Len(t, expired, volRequeueKeys, "ATLAS_VOLUME_ROWS too small for the chunked requeue")
	keys := make([]string, 0, len(expired)+volForeignKeys)
	for _, i := range expired {
		keys = append(keys, volKey(i))
	}
	for _, i := range volKeysOf(n, volForeignKeys, volNotDue) {
		keys = append(keys, volKey(i))
	}
	at := volT0.Add(time.Minute)
	var moved int
	livebackend.Measure(t, "Requeue key list", len(keys), func() {
		var err error
		moved, err = s.Requeue(ctx, at, keys...)
		require.NoError(t, err)
	})
	require.Equal(t, volRequeueKeys, moved)
	livebackend.Measure(t, "Requeue all", counts[volExpired]-volRequeueKeys, func() {
		var err error
		moved, err = s.Requeue(ctx, at)
		require.NoError(t, err)
	})
	require.Equal(t, counts[volExpired]-volRequeueKeys, moved)
	left, err := s.ListByState(ctx, retryqueue.StateExpired, 1)
	require.NoError(t, err)
	require.Empty(t, left)
}

// volCounter counts pending rows by next attempt, reading the backend
// directly: the Storage API has no count.
type volCounter interface {
	pendingAfter(tb testing.TB, t time.Time) int64
	pendingAt(tb testing.TB, t time.Time) int64
}

// volWake wakes every pending row and asserts none is left scheduled after the
// wake instant and the not-due ones now sit at it.
func volWake(t *testing.T, s storagetest.Storage, c volCounter, n int) {
	t.Helper()
	counts := volCounts(n)
	at := volT0.Add(2 * time.Minute)
	livebackend.Measure(t, "WakeDue", counts[volNotDue], func() {
		require.NoError(t, s.WakeDue(t.Context(), at))
	})
	require.Zero(t, c.pendingAfter(t, at))
	require.Equal(t, int64(counts[volNotDue]), c.pendingAt(t, at))
}

// ---- SQL ----

// sqlVolCounter counts through SQL.
type sqlVolCounter struct {
	db    *sql.DB
	table string
}

func (c sqlVolCounter) count(tb testing.TB, where string, at time.Time) int64 {
	tb.Helper()
	var n int64
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE state = 'pending' AND next_attempt_at %s %d", c.table, where, at.UnixNano())
	require.NoError(tb, c.db.QueryRowContext(tb.Context(), q).Scan(&n))
	return n
}

func (c sqlVolCounter) pendingAfter(tb testing.TB, t time.Time) int64 { return c.count(tb, ">", t) }
func (c sqlVolCounter) pendingAt(tb testing.TB, t time.Time) int64    { return c.count(tb, "=", t) }

// sqlVolSeed loads n rows in the column format of the store (store.go
// buildStatements and schema.go): instants as Unix nanoseconds, 0 for the
// zero time, the payload as the JSON codec writes it.
func sqlVolSeed(t *testing.T, db *sql.DB, d sqldb.Dialect, table string, n int) {
	t.Helper()
	payloads := map[volCategory][]byte{}
	for c := volDue; c <= volResolvedNew; c++ {
		b, err := json.Marshal(storagetest.Payload{ValueOn: volT0, Note: fmt.Sprintf("category-%d", c)})
		require.NoError(t, err)
		payloads[c] = b
	}
	text := func(s string) any {
		if d == sqldb.DialectPostgres {
			return s
		}
		return []byte(s)
	}
	nanos := func(t time.Time) int64 {
		if t.IsZero() {
			return 0
		}
		return t.UnixNano()
	}
	columns := []string{"id", "generation", "state", "since", "attempts", "next_attempt_at", "expired_at", "resolved_at",
		"flagged", "flag_reason", "payload"}
	livebackend.Measure(t, "seed", n, func() {
		livebackend.SQLBulkInsert(t, db, string(d), table, columns, n, func(i int) []any {
			reason := ""
			if volFlagged(i) {
				reason = volFlagReason
			}
			return []any{text(volKey(i)), int64(1), text(string(volState(i))), nanos(volSince(i)), int64(0),
				nanos(volNext(i)), nanos(volExpiredAt(i)), nanos(volResolvedAt(i)), volFlagged(i), []byte(reason),
				payloads[volCategoryOf(i)]}
		})
	})
	if d == sqldb.DialectPostgres {
		_, err := db.ExecContext(t.Context(), "ANALYZE "+table)
		require.NoError(t, err)
		return
	}
	_, err := db.ExecContext(t.Context(), "ANALYZE TABLE "+table)
	require.NoError(t, err)
}

// sqlExplain returns the JSON plan of query.
func sqlExplain(tb testing.TB, db *sql.DB, d sqldb.Dialect, query string) string {
	tb.Helper()
	prefix := "EXPLAIN FORMAT=JSON "
	if d == sqldb.DialectPostgres {
		prefix = "EXPLAIN (FORMAT JSON) "
	}
	var plan string
	require.NoError(tb, db.QueryRowContext(tb.Context(), prefix+query).Scan(&plan), query)
	return plan
}

// sqlExplainHotQueries asserts every hot statement of the store reads through
// an index on the seeded table. The shapes are those of buildStatements in
// reliability/retryqueue/storages/sqldb/store.go, with literals for the bound
// values; index names follow schema.go (<table>_<suffix>_idx).
func sqlExplainHotQueries(t *testing.T, db *sql.DB, d sqldb.Dialect, table string) {
	t.Helper()
	const cols = "id, generation, state, since, attempts, next_attempt_at, expired_at, resolved_at, flagged, flag_reason, payload"
	now := volT0.UnixNano()
	nowNanos := "CAST(UNIX_TIMESTAMP() AS SIGNED) * 1000000000"
	if d == sqldb.DialectPostgres {
		nowNanos = "CAST(FLOOR(EXTRACT(EPOCH FROM now())) AS BIGINT) * 1000000000"
	}
	key := "'" + volKey(1) + "'"
	inList := make([]string, 0, volPage)
	for i := range volPage {
		inList = append(inList, "'"+volKey(i)+"'")
	}
	purge := fmt.Sprintf("DELETE FROM %[1]s WHERE id IN (SELECT id FROM %[1]s WHERE state = 'resolved' AND resolved_at <> 0"+
		" AND resolved_at <= %[2]s - %[3]d LIMIT %[4]d FOR UPDATE SKIP LOCKED)", table, nowNanos, volRetention.Nanoseconds(), volPurgeBatch)
	if d != sqldb.DialectPostgres {
		purge = fmt.Sprintf("DELETE FROM %s WHERE state = 'resolved' AND resolved_at <> 0 AND resolved_at <= %s - %d LIMIT %d",
			table, nowNanos, volRetention.Nanoseconds(), volPurgeBatch)
	}
	pg := d == sqldb.DialectPostgres
	// pgScan names a PostgreSQL plan that full-scans the table by the
	// planner's choice; such a query is logged, not asserted, and reported.
	// Drop the reason once the statement reads through an index.
	pgScan := func(reason string) string {
		if pg {
			return reason
		}
		return ""
	}
	queries := []struct{ name, index, knownScan, query string }{
		{"ListDue", "_due_idx", "", fmt.Sprintf("SELECT %s FROM %s WHERE state = 'pending' AND next_attempt_at <= %d"+
			" ORDER BY next_attempt_at, id LIMIT %d", cols, table, now, volPage)},
		// WakeDue updates every pending row scheduled after now, here half the
		// table; PostgreSQL rightly prefers a sequential scan for that share.
		{"WakeDue", "_due_idx", pgScan("not selective: updates every pending row scheduled after now"),
			fmt.Sprintf("UPDATE %s SET next_attempt_at = %d WHERE state = 'pending' AND next_attempt_at > %d", table, now, now)},
		{"ListByState", "_state_idx", "", fmt.Sprintf("SELECT %s FROM %s WHERE state = 'expired' ORDER BY since, id LIMIT %d",
			cols, table, volPage)},
		{"Requeue keys", "", "", fmt.Sprintf("UPDATE %s SET state = 'pending', since = %d, next_attempt_at = %d, attempts = 0,"+
			" expired_at = 0 WHERE state = 'expired' AND id IN (%s)", table, now, now, strings.Join(inList, ", "))},
		// Any index leading with state serves it; PostgreSQL picks either.
		{"Requeue all", "", "", fmt.Sprintf("UPDATE %s SET state = 'pending', since = %d, next_attempt_at = %d,"+
			" attempts = 0, expired_at = 0 WHERE state = 'expired'", table, now, now)},
		// The batch subquery reads _resolved_idx, but PostgreSQL joins its ids
		// back to the table with a hash join over a sequential scan of the
		// whole table instead of primary-key look-ups.
		{"PurgeExpired", "_resolved_idx", pgScan("DELETE ... WHERE id IN (batch subquery) hash-joins over a sequential scan"), purge},
		{"Reschedule", "", "", fmt.Sprintf("UPDATE %s SET attempts = attempts + 1, next_attempt_at = %d"+
			" WHERE id = %s AND state = 'pending' AND generation = 1", table, now, key)},
		{"Get", "", "", fmt.Sprintf("SELECT %s FROM %s WHERE id = %s", cols, table, key)},
	}
	for _, q := range queries {
		plan := sqlExplain(t, db, d, q.query)
		scans := strings.Contains(plan, `"Seq Scan"`)
		if !pg {
			scans = regexp.MustCompile(`"access_type":\s*"ALL"`).MatchString(plan)
		}
		if q.knownScan != "" {
			t.Logf("KNOWN FULL SCAN %s (%s): full scan in plan = %t", q.name, q.knownScan, scans)
			continue
		}
		assert.False(t, scans, "%s full-scans: %s", q.name, plan)
		if q.index != "" {
			assert.Contains(t, plan, q.index, "%s does not use %s: %s", q.name, q.index, plan)
		}
	}
}

// TestVolume_SQL seeds ATLAS_VOLUME_ROWS rows per SQL server and checks the
// plans of the hot statements, a full ListDue sweep, ListByState, the
// chunked and the unrestricted Requeue, WakeDue and PurgeExpired.
func TestVolume_SQL(t *testing.T) {
	n := livebackend.VolumeRows(t)
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			s, db, table := target.store(t, sqldb.WithResolvedRetention(volRetention), sqldb.WithPurgeBatchSize(volPurgeBatch),
				sqldb.WithPurgeInterval(0))
			sqlVolSeed(t, db, target.dialect, table, n)
			sqlExplainHotQueries(t, db, target.dialect, table)

			volSweep(t, s, n)
			volListByState(t, s, n)
			volRequeue(t, s, n)
			volWake(t, s, sqlVolCounter{db: db, table: table}, n)

			counts := volCounts(n)
			var purged int64
			livebackend.Measure(t, "PurgeExpired", counts[volResolvedOld], func() {
				var err error
				purged, err = s.PurgeExpired(t.Context())
				require.NoError(t, err)
			})
			require.Equal(t, int64(counts[volResolvedOld]), purged)
			var resolved int64
			require.NoError(t, db.QueryRowContext(t.Context(),
				"SELECT COUNT(*) FROM "+table+" WHERE state = 'resolved'").Scan(&resolved))
			require.Equal(t, int64(counts[volResolvedNew]+counts[volDue]), resolved)
			again, err := s.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.Zero(t, again)
		})
	}
}

// ---- MongoDB ----

// mongoVolCounter counts through the collection.
type mongoVolCounter struct{ coll *mongodrv.Collection }

func (c mongoVolCounter) count(tb testing.TB, op string, at time.Time) int64 {
	tb.Helper()
	n, err := c.coll.CountDocuments(tb.Context(), bson.M{"state": "pending", "nextAttemptAt": bson.M{op: at}})
	require.NoError(tb, err)
	return n
}

func (c mongoVolCounter) pendingAfter(tb testing.TB, t time.Time) int64 { return c.count(tb, "$gt", t) }
func (c mongoVolCounter) pendingAt(tb testing.TB, t time.Time) int64    { return c.count(tb, "$eq", t) }

// mongoVolSeed loads n documents in the layout of the store (entity.go
// itemDoc, the payload inlined, the default flag fields set only when
// flagged, expiredAt and resolvedAt only when set).
func mongoVolSeed(t *testing.T, coll *mongodrv.Collection, n int) {
	t.Helper()
	livebackend.Measure(t, "seed", n, func() {
		livebackend.MongoBulkInsert(t, coll, n, func(i int) any {
			p := volPayload(i)
			doc := bson.D{
				{Key: "_id", Value: volKey(i)}, {Key: "since", Value: volSince(i)}, {Key: "attempts", Value: 0},
				{Key: "nextAttemptAt", Value: volNext(i)}, {Key: "state", Value: string(volState(i))},
			}
			if at := volExpiredAt(i); !at.IsZero() {
				doc = append(doc, bson.E{Key: "expiredAt", Value: at})
			}
			if at := volResolvedAt(i); !at.IsZero() {
				doc = append(doc, bson.E{Key: "resolvedAt", Value: at})
			}
			doc = append(doc, bson.E{Key: "generation", Value: int64(1)},
				bson.E{Key: "valueOn", Value: p.ValueOn}, bson.E{Key: "note", Value: p.Note})
			if volFlagged(i) {
				doc = append(doc, bson.E{Key: retrymongo.DefaultFlagField, Value: true},
					bson.E{Key: retrymongo.DefaultFlagReasonField, Value: volFlagReason})
			}
			return doc
		})
	})
}

// mongoExplainHotQueries asserts every hot operation of the store reads
// through an index. The filters and sorts are those of
// reliability/retryqueue/storages/mongo/store.go.
func mongoExplainHotQueries(t *testing.T, coll *mongodrv.Collection) {
	t.Helper()
	keys := bson.A{}
	for i := range volPage {
		keys = append(keys, volKey(i))
	}
	find := func(filter, sort bson.D) bson.D {
		return bson.D{{Key: "find", Value: coll.Name()}, {Key: "filter", Value: filter}, {Key: "sort", Value: sort},
			{Key: "limit", Value: volPage}}
	}
	update := func(filter bson.D, set bson.D, multi bool) bson.D {
		return bson.D{{Key: "update", Value: coll.Name()}, {Key: "updates", Value: bson.A{
			bson.D{{Key: "q", Value: filter}, {Key: "u", Value: bson.D{{Key: "$set", Value: set}}}, {Key: "multi", Value: multi}},
		}}}
	}
	queries := []struct {
		name, index string
		cmd         bson.D
	}{
		{"ListDue", retrymongo.IndexPendingNextAttempt, find(
			bson.D{{Key: "state", Value: "pending"}, {Key: "nextAttemptAt", Value: bson.D{{Key: "$lte", Value: volT0}}}},
			bson.D{{Key: "nextAttemptAt", Value: 1}, {Key: "_id", Value: 1}})},
		{"WakeDue", retrymongo.IndexPendingNextAttempt, update(
			bson.D{{Key: "state", Value: "pending"}, {Key: "nextAttemptAt", Value: bson.D{{Key: "$gt", Value: volT0}}}},
			bson.D{{Key: "nextAttemptAt", Value: volT0}}, true)},
		{"ListByState", retrymongo.IndexStateSince, find(bson.D{{Key: "state", Value: "expired"}},
			bson.D{{Key: "since", Value: 1}, {Key: "_id", Value: 1}})},
		{"Requeue keys", "", update(bson.D{{Key: "state", Value: "expired"}, {Key: "_id", Value: bson.D{{Key: "$in", Value: keys}}}},
			bson.D{{Key: "state", Value: "pending"}}, true)},
		{"Requeue all", retrymongo.IndexStateSince, update(bson.D{{Key: "state", Value: "expired"}},
			bson.D{{Key: "state", Value: "pending"}}, true)},
		{"Reschedule", "", update(bson.D{{Key: "_id", Value: volKey(1)}, {Key: "state", Value: "pending"},
			{Key: "generation", Value: int64(1)}}, bson.D{{Key: "nextAttemptAt", Value: volT0}}, false)},
	}
	for _, q := range queries {
		raw, err := coll.Database().RunCommand(t.Context(), bson.D{{Key: "explain", Value: q.cmd},
			{Key: "verbosity", Value: "queryPlanner"}}).Raw()
		require.NoError(t, err, q.name)
		plan, err := raw.LookupErr("queryPlanner", "winningPlan")
		require.NoError(t, err, "%s: no winning plan in %s", q.name, raw)
		winning := plan.String()
		require.NotContains(t, winning, "COLLSCAN", "%s full-scans: %s", q.name, winning)
		require.True(t, strings.Contains(winning, "IXSCAN") || strings.Contains(winning, "IDHACK") ||
			strings.Contains(winning, "EXPRESS"), "%s does not use an index: %s", q.name, winning)
		if q.index != "" {
			require.Contains(t, winning, q.index, "%s does not use %s: %s", q.name, q.index, winning)
		}
	}
}

// TestVolume_Mongo seeds ATLAS_VOLUME_ROWS documents and checks the plans of
// the hot operations, a full ListDue sweep, ListByState, the Requeue of a long
// key list and of every expired item, and WakeDue. Resolved items are purged
// by the TTL monitor, not the store, so there is no purge to measure.
func TestVolume_Mongo(t *testing.T) {
	n := livebackend.VolumeRows(t)
	coll := mongoColl(t)
	s := newStore(t, coll)
	mongoVolSeed(t, coll, n)
	mongoExplainHotQueries(t, coll)

	volSweep(t, s, n)
	volListByState(t, s, n)
	volRequeue(t, s, n)
	volWake(t, s, mongoVolCounter{coll: coll}, n)
}

//go:build volume

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auditit_test

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas-core/time/clock"
	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/sqldb"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	auditmongo "github.com/altessa-s/go-atlas/audit/storages/mongo"
)

// Shape of the volume data set: event i belongs to actor i%volActors and
// resource i%volResources, has a request ID of its own and shares its trace
// with the volTraceSize events around it. Event i is stamped volBase + i ms,
// so (timestamp, ID) order is index order.
const (
	volActors    = 1000
	volResources = 5000
	volTraceSize = 10
	volSeedBatch = 5000
	volPageSize  = 100
)

var volBase = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func volumeEvent(i int) *audit.Event {
	return &audit.Event{
		ID: fmt.Sprintf("ev-%09d", i), Type: "volume", Action: "read", Timestamp: volBase.Add(time.Duration(i) * time.Millisecond),
		Actor:    audit.Actor{ID: fmt.Sprintf("actor-%04d", i%volActors), Type: "user"},
		Resource: audit.Resource{Type: "doc", ID: fmt.Sprintf("res-%05d", i%volResources)},
		Result:   audit.Result{Status: "success"},
		Context:  audit.EventContext{RequestID: fmt.Sprintf("req-%09d", i), TraceID: fmt.Sprintf("trace-%08d", i/volTraceSize)},
	}
}

func volumeID(i int) string { return fmt.Sprintf("ev-%09d", i) }

// seedEvents stores events 0..n-1 through StoreBatch, volSeedBatch at a time.
func seedEvents(t *testing.T, st audit.Storage, n int) {
	t.Helper()
	batch := make([]*audit.Event, 0, volSeedBatch)
	livebackend.Measure(t, "StoreBatch", n, func() {
		for start := 0; start < n; start += volSeedBatch {
			batch = batch[:0]
			for i := start; i < min(start+volSeedBatch, n); i++ {
				batch = append(batch, volumeEvent(i))
			}
			require.NoError(t, st.StoreBatch(t.Context(), batch))
		}
	})
}

// volumeCase is one filtered query over the data set with the exact event
// indexes it must return, in its sort order.
type volumeCase struct {
	name  string
	query audit.Query
	want  []int
	// sqlPreds and sqlArgs are the WHERE predicates sqldb.Storage.where
	// renders for query; sqlKeys the MySQL indexes that may serve it.
	sqlPreds []string
	sqlArgs  []any
	sqlKeys  []string
	// mongoFilter is the filter auditmongo.buildFilter renders for query;
	// mongoIndex the index that must serve it.
	mongoFilter bson.D
	mongoIndex  string
}

// descending returns the indexes below n congruent to rem modulo step, newest
// first.
func descending(n, step, rem int) []int {
	var out []int
	for i := (n-1-rem)/step*step + rem; i >= 0; i -= step {
		if i < n {
			out = append(out, i)
		}
	}
	return out
}

func volumeCases(n int, table string) []volumeCase {
	ix := func(suffix string) string { return livebackend.MySQLIndexName(table, suffix) }
	actor, resource := 7%volActors, 42%volResources
	lo := n / 2
	hi := lo + max(n/100, 1) - 1
	start, end := volBase.Add(time.Duration(lo)*time.Millisecond), volBase.Add(time.Duration(hi)*time.Millisecond)
	var span []int
	for i := lo; i <= hi && i < n; i++ {
		span = append(span, i)
	}
	req := n / 3
	trace := n / 7 / volTraceSize
	var traced []int
	for i := min(trace*volTraceSize+volTraceSize, n) - 1; i >= trace*volTraceSize; i-- {
		traced = append(traced, i)
	}
	var newest []int
	for i := n - 1; i >= max(n-volPageSize/2, 0); i-- {
		newest = append(newest, i)
	}
	actorID, resourceID := fmt.Sprintf("actor-%04d", actor), fmt.Sprintf("res-%05d", resource)
	reqID, traceID := fmt.Sprintf("req-%09d", req), fmt.Sprintf("trace-%08d", trace)
	return []volumeCase{
		{
			name: "actor", query: audit.Query{ActorID: actorID}, want: descending(n, volActors, actor),
			sqlPreds: []string{"actor_id = ?"}, sqlArgs: []any{actorID}, sqlKeys: []string{ix("actor")},
			mongoFilter: bson.D{{Key: "actor.id", Value: actorID}}, mongoIndex: "actor.id_1_timestamp_-1",
		},
		{
			name: "resource", query: audit.Query{ResourceType: "doc", ResourceID: resourceID}, want: descending(n, volResources, resource),
			sqlPreds: []string{"resource_type = ?", "resource_id = ?"}, sqlArgs: []any{"doc", resourceID}, sqlKeys: []string{ix("resource")},
			mongoFilter: bson.D{{Key: "resource.type", Value: "doc"}, {Key: "resource.id", Value: resourceID}},
			mongoIndex:  "resource.type_1_resource.id_1_timestamp_-1",
		},
		{
			name: "time range", query: audit.Query{StartTime: &start, EndTime: &end, SortOrder: audit.SortOrderAsc}, want: span,
			sqlPreds: []string{"ts_ms >= ?", "ts_ms <= ?"}, sqlArgs: []any{start.UnixMilli(), end.UnixMilli()}, sqlKeys: []string{ix("ts")},
			mongoFilter: bson.D{{Key: "timestamp", Value: bson.D{{Key: "$gte", Value: start}, {Key: "$lte", Value: end}}}},
			mongoIndex:  "timestamp_-1__id_-1",
		},
		{
			name: "request", query: audit.Query{RequestID: reqID}, want: []int{req},
			sqlPreds: []string{"request_id = ?"}, sqlArgs: []any{reqID}, sqlKeys: []string{ix("request")},
			mongoFilter: bson.D{{Key: "context.request_id", Value: reqID}}, mongoIndex: "context.request_id_1",
		},
		{
			name: "trace", query: audit.Query{TraceID: traceID}, want: traced,
			sqlPreds: []string{"trace_id = ?"}, sqlArgs: []any{traceID}, sqlKeys: []string{ix("trace")},
			mongoFilter: bson.D{{Key: "context.trace_id", Value: traceID}}, mongoIndex: "context.trace_id_1",
		},
		{
			name: "newest", query: audit.Query{Limit: volPageSize / 2}, want: newest,
			sqlKeys: []string{ix("ts")}, mongoFilter: bson.D{}, mongoIndex: "timestamp_-1__id_-1",
		},
	}
}

// pageIDs reads every event of q, volPageSize at a time, each page continuing
// after the cursor of the previous page's last event, as FetchPage does. A
// query with its own Limit reads one page of that size.
func pageIDs(t *testing.T, st audit.Storage, q audit.Query) []string {
	t.Helper()
	single := q.Limit > 0
	if !single {
		q.Limit = volPageSize
	}
	var out []string
	for {
		var last *audit.Event
		got := 0
		for e, err := range st.Query(t.Context(), &q) {
			require.NoError(t, err)
			out = append(out, e.ID)
			last = e
			got++
		}
		if single || got < q.Limit {
			return out
		}
		c := audit.CursorOf(last)
		q.Cursor = &c
	}
}

func volumeIDs(want []int) []string {
	out := make([]string, len(want))
	for i, w := range want {
		out[i] = volumeID(w)
	}
	return out
}

// checkCases pages through every case through the storage and compares the
// events and the count with the expected ones exactly.
func checkCases(t *testing.T, st audit.Storage, cases []volumeCase) {
	t.Helper()
	for _, c := range cases {
		var got []string
		livebackend.Measure(t, "Query "+c.name, len(c.want), func() { got = pageIDs(t, st, c.query) })
		require.Equal(t, volumeIDs(c.want), got, c.name)
		if c.query.Limit == 0 {
			n, err := st.Count(t.Context(), &c.query)
			require.NoError(t, err)
			require.EqualValues(t, len(c.want), n, c.name)
		}
	}
}

// TestVolume_AuditSQL stores VolumeRows events through StoreBatch, pages
// through every indexed filter and checks the exact result, checks that the
// page, count and purge statements are served by an index, and purges the
// oldest tenth through PurgeExpired.
func TestVolume_AuditSQL(t *testing.T) {
	n := livebackend.VolumeRows(t)
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			// Retention cuts off exactly the oldest tenth: events before
			// volBase + n/10 ms.
			expired := n / 10
			now := volBase.Add(time.Duration(expired)*time.Millisecond + time.Hour)
			st, db, table := target.open(t, sqldb.WithTTL(time.Hour), sqldb.WithClock(clock.NewFake(now)), sqldb.WithPurgeInterval(0))
			seedEvents(t, st, n)
			livebackend.Analyze(t, db, string(target.dialect), table)

			cases := volumeCases(n, table)
			for _, c := range cases {
				explainAuditSQL(t, db, target, table, c)
			}
			checkCases(t, st, cases)

			// Copied from sqldb.Storage.PurgeExpired (audit/storages/sqldb/retention.go).
			cutoff := now.Add(-time.Hour).UnixMilli()
			switch {
			case target.dialect == sqldb.DialectPostgres && n < livebackend.StrictPurgePlanRows:
				// Over a small table PostgreSQL joins the batch to the table
				// with a hash semi join, reading it whole: cheaper than one
				// primary-key probe per row there. Log the plan instead.
				q := sqldialect.Postgres.Bind("EXPLAIN (FORMAT JSON) DELETE FROM "+table+" WHERE id IN (SELECT id FROM "+table+
					" WHERE ts_ms < ? ORDER BY ts_ms, id LIMIT ? FOR UPDATE SKIP LOCKED)", 1)
				var plan string
				require.NoError(t, db.QueryRowContext(t.Context(), q, cutoff, sqldb.DefaultPurgeBatchSize).Scan(&plan))
				t.Logf("purge plan below %d rows, not checked: %s", livebackend.StrictPurgePlanRows, strings.Join(strings.Fields(plan), " "))
			case target.dialect == sqldb.DialectPostgres:
				q := sqldialect.Postgres.Bind("DELETE FROM "+table+" WHERE id IN (SELECT id FROM "+table+
					" WHERE ts_ms < ? ORDER BY ts_ms, id LIMIT ? FOR UPDATE SKIP LOCKED)", 1)
				t.Logf("purge plan indexes: %v", livebackend.ExplainPostgres(t, db, table, q, cutoff, sqldb.DefaultPurgeBatchSize))
			default:
				access, key := livebackend.ExplainMySQL(t, db, table, []string{livebackend.MySQLIndexName(table, "ts")},
					"DELETE FROM "+table+" WHERE ts_ms < ? ORDER BY ts_ms, id LIMIT ?", cutoff, sqldb.DefaultPurgeBatchSize)
				t.Logf("purge plan: type=%s key=%s", access, key)
			}

			var purged int64
			livebackend.Measure(t, "PurgeExpired", expired, func() {
				var err error
				purged, err = st.PurgeExpired(t.Context())
				require.NoError(t, err)
			})
			require.EqualValues(t, expired, purged)
			left, err := st.Count(t.Context(), &audit.Query{})
			require.NoError(t, err)
			require.EqualValues(t, n-expired, left)
			require.Equal(t, []string{volumeID(expired)}, pageIDs(t, st, audit.Query{Limit: 1, SortOrder: audit.SortOrderAsc}),
				"the oldest event left is the first one at the cutoff")
			purged, err = st.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.Zero(t, purged)
		})
	}
}

// explainAuditSQL explains the first page, a cursor page and the count of c,
// rendered as sqldb.Storage.Query and Count render them
// (audit/storages/sqldb/sqldb.go).
func explainAuditSQL(t *testing.T, db *sql.DB, target sqlTarget, table string, c volumeCase) {
	t.Helper()
	dir, op := "DESC", "<"
	if c.query.SortOrder == audit.SortOrderAsc {
		dir, op = "ASC", ">"
	}
	where := func(preds []string) string {
		if len(preds) == 0 {
			return ""
		}
		return " WHERE " + strings.Join(preds, " AND ")
	}
	order := " ORDER BY ts_ms " + dir + ", id " + dir + " LIMIT ?"
	// The cursor of a later page: the middle of the expected events.
	mid := c.want[len(c.want)/2]
	ms := volBase.Add(time.Duration(mid) * time.Millisecond).UnixMilli()
	cursorPreds := append(slices.Clone(c.sqlPreds), "(ts_ms "+op+" ? OR (ts_ms = ? AND id "+op+" ?))")
	stmts := []struct {
		label string
		query string
		args  []any
	}{
		{"page", "SELECT payload FROM " + table + where(c.sqlPreds) + order, append(slices.Clone(c.sqlArgs), volPageSize)},
		{"cursor page", "SELECT payload FROM " + table + where(cursorPreds) + order,
			append(slices.Clone(c.sqlArgs), ms, ms, volumeID(mid), volPageSize)},
	}
	if len(c.sqlPreds) > 0 {
		stmts = append(stmts, struct {
			label string
			query string
			args  []any
		}{"count", "SELECT COUNT(*) FROM " + table + where(c.sqlPreds), c.sqlArgs})
	}
	for _, s := range stmts {
		if target.dialect == sqldb.DialectPostgres {
			t.Logf("%s %s plan indexes: %v", c.name, s.label,
				livebackend.ExplainPostgres(t, db, table, sqldialect.Postgres.Bind(s.query, 1), s.args...))
			continue
		}
		access, key := livebackend.ExplainMySQL(t, db, table, c.sqlKeys, s.query, s.args...)
		t.Logf("%s %s plan: type=%s key=%s", c.name, s.label, access, key)
	}
}

// TestVolume_AuditMongo stores VolumeRows events through StoreBatch, pages
// through every filter and checks the exact result, and checks that the find
// and count commands are served by an index. The MongoDB storage has no
// PurgeExpired: retention is the server's TTL index.
func TestVolume_AuditMongo(t *testing.T) {
	n := livebackend.VolumeRows(t)
	db := livebackend.MongoDatabase(t, "auditvol")
	st, err := auditmongo.New(t.Context(), db)
	require.NoError(t, err)
	seedEvents(t, st, n)

	cases := volumeCases(n, "")
	collation := bson.D{{Key: "locale", Value: "simple"}}
	coll := auditmongo.DefaultCollectionName
	for _, c := range cases {
		// Copied from auditmongo.Storage.Query and Count (audit/storages/mongo/mongo.go).
		order := -1
		if c.query.SortOrder == audit.SortOrderAsc {
			order = 1
		}
		cmds := map[string]bson.D{
			"find": {
				{Key: "find", Value: coll}, {Key: "filter", Value: c.mongoFilter},
				{Key: "sort", Value: bson.D{{Key: "timestamp", Value: order}, {Key: "_id", Value: order}}},
				{Key: "collation", Value: collation}, {Key: "limit", Value: volPageSize},
			},
			"count": {{Key: "count", Value: coll}, {Key: "query", Value: c.mongoFilter}, {Key: "collation", Value: collation}},
		}
		if len(c.mongoFilter) == 0 {
			delete(cmds, "count") // an unfiltered count reads the collection metadata
		}
		for label, cmd := range cmds {
			indexes := livebackend.ExplainMongo(t, db, cmd)
			require.Contains(t, indexes, c.mongoIndex, "%s %s", c.name, label)
			t.Logf("%s %s plan indexes: %v", c.name, label, indexes)
		}
	}
	checkCases(t, st, cases)
}

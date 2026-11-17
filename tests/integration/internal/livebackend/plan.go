// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package livebackend

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// StrictPurgePlanRows is the seeded size from which a PostgreSQL purge DELETE
// must not read the table sequentially. Over a smaller table PostgreSQL joins
// the batch to the table with a hash semi join that reads it whole, cheaper
// there than one primary-key probe per row, so that plan is only logged.
const StrictPurgePlanRows = 100_000

// Analyze refreshes the planner statistics of table, as written in SQL, after
// seeding: ANALYZE on [SQLDialectPostgres], ANALYZE TABLE otherwise, whose
// result rows are drained before the connection is reused.
func Analyze(tb testing.TB, db *sql.DB, dialect, table string) {
	tb.Helper()
	stmt := "ANALYZE TABLE " + table
	if dialect == SQLDialectPostgres {
		stmt = "ANALYZE " + table
	}
	rows, err := db.QueryContext(tb.Context(), stmt)
	require.NoError(tb, err)
	for rows.Next() {
	}
	require.NoError(tb, rows.Err())
	require.NoError(tb, rows.Close())
}

// MySQLIndexName is the unquoted name sqldialect.MySQL gives the index suffix
// of table, as EXPLAIN reports it.
func MySQLIndexName(table, suffix string) string {
	return strings.Trim(sqldialect.MySQL.IndexName(table, suffix), "`")
}

// ExplainSQL explains query, written with ? placeholders, on the dialect of
// style and logs the indexes it reads. On PostgreSQL the plan must include
// index; on MySQL index is the expected key [ExplainMySQL] checks. An empty
// index skips the PostgreSQL check, as for an upsert, whose plan names the
// primary key only as its conflict arbiter.
func ExplainSQL(tb testing.TB, db *sql.DB, style sqldialect.Style, table, index, query string, args ...any) {
	tb.Helper()
	if style.Numbered {
		indexes := ExplainPostgres(tb, db, table, style.Bind(query, 1), args...)
		tb.Logf("%s: indexes %v", query, indexes)
		if index != "" {
			require.Contains(tb, indexes, index)
		}
		return
	}
	access, key := ExplainMySQL(tb, db, table, []string{index}, query, args...)
	tb.Logf("%s: access %s, key %s", query, access, key)
}

// ExplainMongoFind checks that a find with filter on coll uses an index, and
// index when it is not empty.
func ExplainMongoFind(tb testing.TB, coll *mongo.Collection, filter bson.M, index string) {
	tb.Helper()
	indexes := ExplainMongo(tb, coll.Database(), bson.D{{Key: "find", Value: coll.Name()}, {Key: "filter", Value: filter}})
	tb.Logf("MongoDB filter %v: indexes %v", filter, indexes)
	if index != "" {
		require.Contains(tb, indexes, index)
	}
}

// ExplainPostgresPurge checks a PostgreSQL purge that deletes by column the
// rows batch selects FOR UPDATE SKIP LOCKED: the batch select must use index
// at any size, and the DELETE must not read the table sequentially from
// [StrictPurgePlanRows] seeded rows (n) on; below that its plan is logged.
func ExplainPostgresPurge(tb testing.TB, db *sql.DB, table, index, column, batch string, n int, args ...any) {
	tb.Helper()
	inner := batch + " FOR UPDATE SKIP LOCKED"
	indexes := ExplainPostgres(tb, db, table, sqldialect.Postgres.Bind(inner, 1), args...)
	require.Contains(tb, indexes, index, "the purge batch select must use %s", index)
	del := sqldialect.Postgres.Bind("DELETE FROM "+sqldialect.Postgres.Ident(table)+" WHERE "+column+" IN ("+inner+")", 1)
	if n >= StrictPurgePlanRows {
		tb.Logf("purge plan indexes: %v", ExplainPostgres(tb, db, table, del, args...))
		return
	}
	var plan string
	require.NoError(tb, db.QueryRowContext(tb.Context(), "EXPLAIN (FORMAT JSON) "+del, args...).Scan(&plan))
	tb.Logf("purge plan below %d rows, not checked: %s", StrictPurgePlanRows, strings.Join(strings.Fields(plan), " "))
}

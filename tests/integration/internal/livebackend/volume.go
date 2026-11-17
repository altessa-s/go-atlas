// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package livebackend

import (
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// DefaultVolumeRows is the number of rows a volume test seeds per backend
// when ATLAS_VOLUME_ROWS is not set.
const DefaultVolumeRows = 1_000_000

// maxSQLParams keeps one multi-row INSERT under PostgreSQL's limit of 65535
// bind parameters (MySQL's is the same).
const maxSQLParams = 60_000

// maxSQLBatchRows caps the rows of one multi-row INSERT.
const maxSQLBatchRows = 1000

// mongoBatchDocs is the number of documents of one InsertMany.
const mongoBatchDocs = 5000

// VolumeRows returns the number of rows a volume test seeds per backend:
// ATLAS_VOLUME_ROWS when set, otherwise [DefaultVolumeRows]. A value that is
// not a positive integer fails the test.
func VolumeRows(tb testing.TB) int {
	tb.Helper()
	v := os.Getenv("ATLAS_VOLUME_ROWS")
	if v == "" {
		return DefaultVolumeRows
	}
	n, err := strconv.Atoi(v)
	require.NoError(tb, err, "ATLAS_VOLUME_ROWS must be an integer")
	require.Positive(tb, n, "ATLAS_VOLUME_ROWS must be positive")
	return n
}

// Measure runs fn, logs how long it took for n items and the resulting rate,
// and returns the duration.
func Measure(tb testing.TB, label string, n int, fn func()) time.Duration {
	tb.Helper()
	start := time.Now()
	fn()
	d := time.Since(start)
	tb.Logf("%s: %d in %s (%.0f/s)", label, n, d.Round(time.Millisecond), float64(n)/max(d.Seconds(), 1e-9))
	return d
}

// SQLBulkInsert inserts n rows into table with multi-row INSERT statements,
// each in its own implicit transaction. row returns the values of row i in the
// order of columns. Placeholders are $n for dialect "postgres" and ? otherwise.
// It is a seeding helper: it bypasses the store under test to load volume fast.
func SQLBulkInsert(tb testing.TB, db *sql.DB, dialect, table string, columns []string, n int, row func(i int) []any) {
	tb.Helper()
	batch := min(maxSQLBatchRows, maxSQLParams/len(columns))
	head := "INSERT INTO " + table + " (" + strings.Join(columns, ", ") + ") VALUES "
	args := make([]any, 0, batch*len(columns))
	var b strings.Builder
	for start := 0; start < n; start += batch {
		end := min(start+batch, n)
		b.Reset()
		args = args[:0]
		b.WriteString(head)
		for i := start; i < end; i++ {
			if i > start {
				b.WriteString(", ")
			}
			b.WriteByte('(')
			for c := range columns {
				if c > 0 {
					b.WriteString(", ")
				}
				if dialect == "postgres" {
					b.WriteString("$" + strconv.Itoa(len(args)+c+1))
				} else {
					b.WriteByte('?')
				}
			}
			b.WriteByte(')')
			vals := row(i)
			require.Len(tb, vals, len(columns), "row %d", i)
			args = append(args, vals...)
		}
		_, err := db.ExecContext(tb.Context(), b.String(), args...)
		require.NoError(tb, err, "bulk insert rows %d..%d into %s", start, end, table)
	}
}

// MongoBulkInsert inserts n documents into coll with unordered InsertMany
// calls. doc returns document i. Like [SQLBulkInsert] it is a seeding helper.
func MongoBulkInsert(tb testing.TB, coll *mongo.Collection, n int, doc func(i int) any) {
	tb.Helper()
	docs := make([]any, 0, mongoBatchDocs)
	for start := 0; start < n; start += mongoBatchDocs {
		end := min(start+mongoBatchDocs, n)
		docs = docs[:0]
		for i := start; i < end; i++ {
			docs = append(docs, doc(i))
		}
		_, err := coll.InsertMany(tb.Context(), docs)
		require.NoError(tb, err, "bulk insert documents %d..%d into %s", start, end, coll.Name())
	}
}

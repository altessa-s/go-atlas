// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package livebackend

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/topology"

	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoConnectTimeout bounds server selection and the ping of [MongoDatabase].
const mongoConnectTimeout = 3 * time.Second

// ExplainPostgres runs EXPLAIN (FORMAT JSON) of query on db and fails the test
// when the plan reads table with a sequential scan. It returns the indexes the
// plan uses, for logging. Run ANALYZE on the table after seeding it first, so
// the planner sees its real size.
func ExplainPostgres(tb testing.TB, db *sql.DB, table, query string, args ...any) []string {
	tb.Helper()
	var raw []byte
	require.NoError(tb, db.QueryRowContext(tb.Context(), "EXPLAIN (FORMAT JSON) "+query, args...).Scan(&raw), "explain %s", query)
	var plans []struct {
		Plan map[string]any `json:"Plan"`
	}
	require.NoError(tb, json.Unmarshal(raw, &plans))
	require.NotEmpty(tb, plans)
	var indexes []string
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		if node["Node Type"] == "Seq Scan" {
			require.NotEqual(tb, table, node["Relation Name"], "sequential scan of %s in the plan of %s:\n%s", table, query, raw)
		}
		if name, ok := node["Index Name"].(string); ok {
			indexes = append(indexes, name)
		}
		children, _ := node["Plans"].([]any)
		for _, c := range children {
			if child, ok := c.(map[string]any); ok {
				walk(child)
			}
		}
	}
	walk(plans[0].Plan)
	return indexes
}

// ExplainMySQL runs EXPLAIN of query on a MySQL or MariaDB db and fails the
// test unless table is read through one of keys, without a full table scan
// (access type ALL). Keys are unquoted index names; "PRIMARY" is the primary
// key. It returns the access type and key of the table's row, for logging.
func ExplainMySQL(tb testing.TB, db *sql.DB, table string, keys []string, query string, args ...any) (access, key string) {
	tb.Helper()
	rows, err := db.QueryContext(tb.Context(), "EXPLAIN "+query, args...)
	require.NoError(tb, err, "explain %s", query)
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	require.NoError(tb, err)
	var plan []string
	found := false
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		require.NoError(tb, rows.Scan(ptrs...))
		row := make(map[string]string, len(cols))
		for i, c := range cols {
			row[strings.ToLower(c)] = vals[i].String
		}
		plan = append(plan, row["table"]+": type="+row["type"]+" key="+row["key"]+" rows="+row["rows"]+" extra="+row["extra"])
		if row["table"] != table {
			continue
		}
		found = true
		access, key = row["type"], row["key"]
	}
	require.NoError(tb, rows.Err())
	require.True(tb, found, "no row for %s in the plan of %s:\n%s", table, query, strings.Join(plan, "\n"))
	require.NotEqual(tb, "ALL", access, "full scan of %s in the plan of %s:\n%s", table, query, strings.Join(plan, "\n"))
	require.Contains(tb, keys, key, "unexpected key for %s in the plan of %s:\n%s", table, query, strings.Join(plan, "\n"))
	return access, key
}

// ExplainMongo runs the explain command for cmd (a find, count, delete, ...
// command document) at queryPlanner verbosity and fails the test when the
// winning plan scans the collection (COLLSCAN) or uses no index (IXSCAN or an
// equivalent index stage). It returns the index names of the winning plan.
func ExplainMongo(tb testing.TB, db *mongo.Database, cmd bson.D) []string {
	tb.Helper()
	var out bson.M
	require.NoError(tb, db.RunCommand(tb.Context(), bson.D{
		{Key: "explain", Value: cmd}, {Key: "verbosity", Value: "queryPlanner"},
	}).Decode(&out), "explain %v", cmd)
	planner, ok := out["queryPlanner"].(bson.D)
	require.True(tb, ok, "explain of %v has no queryPlanner: %v", cmd, out)
	var winning any
	for _, e := range planner {
		if e.Key == "winningPlan" {
			winning = e.Value
		}
	}
	require.NotNil(tb, winning, "explain of %v has no winning plan: %v", cmd, planner)
	var stages, indexes []string
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			for _, e := range n {
				switch s, isString := e.Value.(string); {
				case e.Key == "stage" && isString:
					stages = append(stages, s)
				case e.Key == "indexName" && isString:
					indexes = append(indexes, s)
				default:
					walk(e.Value)
				}
			}
		case bson.A:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(winning)
	require.NotContains(tb, stages, "COLLSCAN", "collection scan in the plan of %v: %v", cmd, winning)
	require.True(tb, slices.ContainsFunc(stages, func(s string) bool {
		return s == "IXSCAN" || s == "EXPRESS_IXSCAN" || s == "IDHACK" || s == "EXPRESS_IDHACK" || s == "COUNT_SCAN" || s == "DISTINCT_SCAN"
	}), "no index stage in the plan of %v: %v", cmd, winning)
	return indexes
}

// MongoDatabase connects to [MongoURI] and returns a database unique to the
// test (see [UniqueName]), dropped on cleanup. It skips the test when the
// server is unreachable.
func MongoDatabase(tb testing.TB, prefix string) *mongo.Database {
	tb.Helper()
	client, err := mongo.Connect(mongoopts.Client().ApplyURI(MongoURI()).SetServerSelectionTimeout(mongoConnectTimeout))
	if err != nil {
		tb.Fatalf("MongoDB configuration failed (%T)", err)
	}
	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), mongoConnectTimeout)
		defer cancel()
		_ = client.Disconnect(ctx)
	})
	ctx, cancel := context.WithTimeout(tb.Context(), mongoConnectTimeout)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		if os.Getenv("MONGO_URI") == "" && os.Getenv("MONGO_PORT") == "" && mongoUnreachable(err) {
			tb.Skip("MongoDB unreachable — start it with: make integration-up")
		}
		tb.Fatalf("MongoDB ping failed (%T)", err)
	}
	db := client.Database(UniqueName(tb, prefix))
	tb.Cleanup(func() {
		ctx := context.WithoutCancel(tb.Context())
		_ = db.Drop(ctx)
	})
	return db
}

// Mongo reports the individual dial failures in the topology attached to its selection timeout.
func mongoUnreachable(err error) bool {
	selection, ok := errors.AsType[topology.ServerSelectionError](err)
	if !ok || len(selection.Desc.Servers) == 0 {
		return SkipEndpoint(err, "")
	}
	for _, s := range selection.Desc.Servers {
		if !SkipEndpoint(s.LastError, "") {
			return false
		}
	}
	return true
}

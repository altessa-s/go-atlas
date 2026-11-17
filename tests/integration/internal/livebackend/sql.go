// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package livebackend

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// SQL dialect names of [SQLTarget.Dialect]. Every SQL storage of the toolkit
// spells its own Dialect type with these values, so a suite converts with
// sqldb.Dialect(target.Dialect).
const (
	SQLDialectPostgres = "postgres"
	SQLDialectMySQL    = "mysql"
)

// database/sql driver names of the SQL targets.
const (
	pgxDriver   = "pgx"
	mysqlDriver = "mysql"
)

// maxTableName leaves room under the shorter of PostgreSQL's (63) and MySQL's
// (64) identifier limits for the index names the stores derive from the table
// name, such as "<table>_published_idx"; a longer table makes a store shorten
// those names.
const maxTableName = 63 - len("_published_idx")

// uniqueNameOverhead is what [uniqueName] adds around the test-name part: two
// underscores and the 8-character random suffix.
const uniqueNameOverhead = 2 + 8

// sqlDDLTimeout bounds the ping and the cleanup DROP of a SQL target.
const sqlDDLTimeout = 3 * time.Second

// SQLTarget is one database/sql server of the docker stack.
type SQLTarget struct {
	// Name is the server's subtest name: postgres, mariadb or mysql.
	Name string
	// Driver is the database/sql driver name; the suite imports the driver.
	Driver string
	// Env names the environment variable that overrides DefaultDSN.
	Env string
	// DefaultDSN reaches the server of tests/integration/docker-compose.yml.
	DefaultDSN string
	// Dialect is [SQLDialectPostgres] or [SQLDialectMySQL].
	Dialect string
}

// SQLTargets returns the SQL servers of the docker stack: PostgreSQL through
// pgx, MariaDB and MySQL through go-sql-driver/mysql.
func SQLTargets() []SQLTarget {
	return []SQLTarget{
		{"postgres", pgxDriver, "POSTGRES_DSN", "postgres://atlas:atlas@127.0.0.1:15432/atlas?sslmode=disable", SQLDialectPostgres},
		{"mariadb", mysqlDriver, "MARIADB_DSN", "atlas:atlas@tcp(127.0.0.1:13306)/atlas", SQLDialectMySQL},
		{"mysql", mysqlDriver, "MYSQL_DSN", "atlas:atlas@tcp(127.0.0.1:13307)/atlas", SQLDialectMySQL},
	}
}

// DSN returns the environment override of the target, or its DefaultDSN.
func (t SQLTarget) DSN() string {
	if v := os.Getenv(t.Env); v != "" {
		return v
	}
	return t.DefaultDSN
}

// Open connects to the server and returns a connection pool, closed on
// cleanup. It skips the test when the server is unreachable; the DSN is not
// printed, as an override may carry a password.
func (t SQLTarget) Open(tb testing.TB) *sql.DB {
	tb.Helper()
	db, err := sql.Open(t.Driver, t.DSN())
	require.NoError(tb, err)
	ctx, cancel := context.WithTimeout(tb.Context(), sqlDDLTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		tb.Skipf("%s unreachable (%v) — start it with: make integration-up", t.Name, err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	return db
}

// SQLTable returns a table name unique to the test (see [UniqueName]), with
// the test-name part shortened to keep it within maxTableName, and
// drops that table from db on cleanup, before db is closed when db came from
// [SQLTarget.Open]. The name is lower case, because the DROP is unquoted and
// PostgreSQL folds an unquoted name; a prefix that would make it exceed every
// server's identifier limit fails the test.
func SQLTable(tb testing.TB, db *sql.DB, prefix string) string {
	tb.Helper()
	// The prefix, two underscores and the random suffix stay whole.
	table := uniqueName(tb, prefix, maxTableName-len(prefix)-uniqueNameOverhead)
	require.LessOrEqual(tb, len(table), maxTableName, "shorten the table prefix %q", prefix)
	tb.Cleanup(func() {
		// The test context is canceled before cleanup callbacks run.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), sqlDDLTimeout)
		defer cancel()
		_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table)
		require.NoError(tb, err)
	})
	return table
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// Dialect selects the SQL flavor the store speaks.
type Dialect string

const (
	// DialectPostgres targets PostgreSQL 12 or newer.
	DialectPostgres Dialect = "postgres"
	// DialectMySQL targets MySQL 8.0+ and MariaDB 10.6+.
	DialectMySQL Dialect = "mysql"
)

// Sentinel errors returned by the store.
var (
	// ErrUnsupportedDialect is returned by [New] for a dialect other than
	// [DialectPostgres] or [DialectMySQL].
	ErrUnsupportedDialect = sqldialect.ErrUnsupportedDialect
	// ErrInvalidTableName is returned by [New] when the table name is not a
	// plain SQL identifier, optionally qualified by one schema name, or a part
	// of it exceeds 63 characters.
	ErrInvalidTableName = sqldialect.ErrInvalidTableName
	// ErrValueTooLong is returned by Mark, Reopen and Move for a key longer
	// than [MaxKeyLength] characters. Rejecting it up front matters because
	// PostgreSQL silently trims excess trailing spaces, which would make "k"
	// and "k " (past the limit) the same row.
	ErrValueTooLong = errors.New("sqldb: value exceeds its column length")
)

// MaxKeyLength bounds an item key, in characters.
const MaxKeyLength = 255

// Server error codes the store recognizes: a unique-key violation.
const (
	// sqlStateUniqueViolation is PostgreSQL's unique_violation SQLSTATE.
	sqlStateUniqueViolation = "23505"
	// mysqlDuplicateKey is MySQL's ER_DUP_ENTRY error number.
	mysqlDuplicateKey = 1062
)

// dialect holds the per-flavor differences: identifier quoting, placeholder
// style, how strings are bound, and the database clock.
type dialect struct {
	sqldialect.Style
	name Dialect
	// nowNanos renders the database clock in Unix nanoseconds, truncated to
	// whole seconds, as a signed integer.
	nowNanos string
}

func dialectFor(d Dialect) (dialect, error) {
	switch d {
	case DialectPostgres:
		return dialect{Style: sqldialect.Postgres, name: d,
			nowNanos: "CAST(FLOOR(EXTRACT(EPOCH FROM now())) AS BIGINT) * 1000000000"}, nil
	case DialectMySQL:
		// UNIX_TIMESTAMP() is unsigned and independent of the session time
		// zone; the cast keeps the cutoff arithmetic signed.
		return dialect{Style: sqldialect.MySQL, name: d, nowNanos: "CAST(UNIX_TIMESTAMP() AS SIGNED) * 1000000000"}, nil
	default:
		return dialect{}, fmt.Errorf("%w: %q", ErrUnsupportedDialect, d)
	}
}

// bind rewrites the ? placeholders of a statement written by this package into
// the dialect's form, numbering from 1.
func (d dialect) bind(query string) string {
	return d.Bind(query, 1)
}

// textArg binds a string for a text column: as-is on PostgreSQL, as bytes on
// MySQL, whose columns are binary so no character-set conversion can touch the
// value.
func (d dialect) textArg(s string) any {
	if d.name == DialectPostgres {
		return s
	}
	return []byte(s)
}

// sqlStateError is the shape of the PostgreSQL drivers' server error.
type sqlStateError interface {
	error
	SQLState() string
}

// isDuplicateKey reports whether err is a unique-key violation. The package
// imports no SQL driver, so the code is read by shape: pgx and lib/pq expose
// the SQLSTATE through a SQLState method, go-sql-driver/mysql exposes the
// server error number only as the Number field of its error type.
func isDuplicateKey(err error) bool {
	if e, ok := errors.AsType[sqlStateError](err); ok {
		return e.SQLState() == sqlStateUniqueViolation
	}
	return mysqlErrorNumber(err) == mysqlDuplicateKey
}

// mysqlErrorNumber returns the Number field of the first error in err's chain
// that has one, as go-sql-driver/mysql's MySQLError does; 0 when none does.
func mysqlErrorNumber(err error) uint16 {
	for ; err != nil; err = errors.Unwrap(err) {
		v := reflect.Indirect(reflect.ValueOf(err))
		if v.Kind() != reflect.Struct {
			continue
		}
		if f := v.FieldByName("Number"); f.IsValid() && f.Kind() == reflect.Uint16 {
			return uint16(f.Uint()) //nolint:gosec // read from a uint16
		}
	}
	return 0
}

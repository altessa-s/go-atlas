// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"fmt"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/sqlident"
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
	ErrInvalidTableName = sqlident.ErrInvalidTableName
	// ErrValueTooLong is returned by Mark, Reopen and Move for a key longer
	// than [MaxKeyLength] characters. Rejecting it up front matters because
	// PostgreSQL silently trims excess trailing spaces, which would make "k"
	// and "k " (past the limit) the same row.
	ErrValueTooLong = sqldialect.ErrValueTooLong
)

// MaxKeyLength bounds an item key, in characters.
const MaxKeyLength = 255

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

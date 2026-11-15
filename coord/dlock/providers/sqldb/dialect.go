// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// Dialect selects the SQL flavor the locker speaks.
type Dialect string

const (
	// DialectPostgres targets PostgreSQL 12 or newer.
	DialectPostgres Dialect = "postgres"
	// DialectMySQL targets MySQL 8.0+ and MariaDB 10.6+.
	DialectMySQL Dialect = "mysql"
)

// Sentinel errors returned by the locker.
var (
	// ErrUnsupportedDialect is returned by [New] for a dialect other than
	// [DialectPostgres] or [DialectMySQL].
	ErrUnsupportedDialect = sqldialect.ErrUnsupportedDialect
	// ErrInvalidTableName is returned by [New] when the table name is not a
	// plain SQL identifier, optionally qualified by one schema name, or a part
	// of it exceeds 63 characters.
	ErrInvalidTableName = sqldialect.ErrInvalidTableName
	// ErrValueTooLong is returned by Lock for a key longer than [MaxKeyLength]
	// characters. Rejecting it up front matters because PostgreSQL silently
	// trims excess trailing spaces, which would make "k" and "k " (past the
	// limit) the same lock.
	ErrValueTooLong = errors.New("sqldb: value exceeds its column length")
)

// MaxKeyLength bounds a lock key, in characters.
const MaxKeyLength = 255

// dialect holds the per-flavor differences: identifier quoting, placeholder
// style, the server clock, and how strings are bound.
type dialect struct {
	sqldialect.Style
	name Dialect
	// now renders the server's current time in Unix microseconds, read when
	// the expression is evaluated rather than at transaction start, so a lease
	// decision taken after a lock wait compares against the actual time.
	now string
}

func dialectFor(d Dialect) (dialect, error) {
	switch d {
	case DialectPostgres:
		return dialect{Style: sqldialect.Postgres, name: d,
			now: "CAST(EXTRACT(EPOCH FROM clock_timestamp()) * 1000000 AS BIGINT)"}, nil
	case DialectMySQL:
		// DATETIME arithmetic on UTC values, so neither the session time zone
		// nor a DST fold can shift the result.
		return dialect{Style: sqldialect.MySQL, name: d,
			now: "TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', UTC_TIMESTAMP(6))"}, nil
	default:
		return dialect{}, fmt.Errorf("%w: %q", ErrUnsupportedDialect, d)
	}
}

// textArg binds a string for a text column: as-is on PostgreSQL (TEXT), as
// bytes on MySQL, whose columns are binary so no character-set conversion can
// touch the value.
func (d dialect) textArg(s string) any {
	if d.name == DialectPostgres {
		return s
	}
	return []byte(s)
}

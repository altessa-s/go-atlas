// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// Dialect selects the SQL flavor the store speaks.
type Dialect string

const (
	// DialectPostgres targets PostgreSQL 12 or newer.
	DialectPostgres Dialect = "postgres"
	// DialectMySQL targets MySQL 8.0+ and MariaDB 10.6+ (both support
	// FOR UPDATE SKIP LOCKED).
	DialectMySQL Dialect = "mysql"
)

// Sentinel errors returned by [New].
var (
	// ErrUnsupportedDialect is returned for a dialect other than
	// [DialectPostgres] or [DialectMySQL].
	ErrUnsupportedDialect = sqldialect.ErrUnsupportedDialect
	// ErrInvalidTableName is returned when the table name is not a plain SQL
	// identifier, optionally qualified by one schema name, or a part of it
	// exceeds 63 characters.
	ErrInvalidTableName = sqldialect.ErrInvalidTableName
)

// mysqlTimeLayout renders a DATETIME(6) value. Timestamps cross the MySQL wire
// as UTC strings in this layout, never as time.Time: go-sql-driver/mysql
// converts a bound time.Time into its configured loc and parses DATETIME in
// that loc, which would shift every instant for a DSN with loc != UTC.
const mysqlTimeLayout = "2006-01-02 15:04:05.000000"

// dialect holds the per-flavor differences: placeholders, quoting, the database
// clock, interval arithmetic, and how timestamps cross the wire.
type dialect struct {
	sqldialect.Style
	name Dialect
	now  string // the backend clock
}

func dialectFor(d Dialect) (dialect, error) {
	switch d {
	case DialectPostgres:
		// now() is the transaction start time, so a fetch transaction compares
		// and stamps against one instant — like MongoDB's $$NOW.
		return dialect{Style: sqldialect.Postgres, name: d, now: "now()"}, nil
	case DialectMySQL:
		return dialect{Style: sqldialect.MySQL, name: d, now: "UTC_TIMESTAMP(6)"}, nil
	default:
		return dialect{}, fmt.Errorf("%w: %q", ErrUnsupportedDialect, d)
	}
}

// bind rewrites the ? placeholders of a statement written by this package into
// the dialect's form, numbering from 1.
func (d dialect) bind(query string) string {
	return d.Bind(query, 1)
}

// shifted renders the backend clock moved by a bound duration: "now - ?" or
// "now + ?". Pair it with [dialect.duration] for the argument.
func (d dialect) shifted(op string) string {
	if d.name == DialectPostgres {
		return "(" + d.now + " " + op + " make_interval(secs => ?))"
	}
	return "(" + d.now + " " + op + " INTERVAL ? MICROSECOND)"
}

// duration is the bound argument for [dialect.shifted].
func (d dialect) duration(v time.Duration) any {
	if d.name == DialectPostgres {
		return v.Seconds()
	}
	return v.Microseconds()
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

// nullableBytes binds an optional string for a byte column (NULL when nil).
func nullableBytes(s *string) any {
	if s == nil {
		return nil
	}
	return []byte(*s)
}

// timeArg binds an instant, or NULL for the zero time.
func (d dialect) timeArg(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	if d.name == DialectPostgres {
		return t.UTC()
	}
	return t.UTC().Format(mysqlTimeLayout)
}

// timeCol selects a timestamp column in the form [dialect.timeDest] reads.
func (d dialect) timeCol(col string) string {
	if d.name == DialectPostgres {
		return col
	}
	return "DATE_FORMAT(" + col + ", '%Y-%m-%d %H:%i:%s.%f')"
}

// timeScan holds one [dialect.timeCol] column while a row is scanned: the
// native value on PostgreSQL, the DATE_FORMAT string on MySQL.
type timeScan struct {
	t sql.NullTime
	s sql.NullString
}

// timeDest returns the scan destination for a [dialect.timeCol] column.
func (d dialect) timeDest(v *timeScan) any {
	if d.name == DialectPostgres {
		return &v.t
	}
	return &v.s
}

// timeValue converts a column scanned through [dialect.timeDest] to a time
// (zero for NULL).
func (d dialect) timeValue(v *timeScan) (time.Time, error) {
	if d.name == DialectPostgres {
		if !v.t.Valid {
			return time.Time{}, nil
		}
		return v.t.Time.UTC(), nil
	}
	if !v.s.Valid {
		return time.Time{}, nil
	}
	return time.ParseInLocation(mysqlTimeLayout, v.s.String, time.UTC)
}

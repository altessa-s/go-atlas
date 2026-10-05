// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	ErrUnsupportedDialect = errors.New("sqldb: unsupported dialect")
	// ErrInvalidTableName is returned when the table name is not a plain SQL
	// identifier, optionally qualified by one schema name.
	ErrInvalidTableName = errors.New("sqldb: invalid table name")
)

// identifier matches a plain SQL identifier with an optional schema qualifier.
// The table name is the only SQL fragment that cannot be bound as a parameter,
// so it is restricted to this shape and then quoted.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// maxIdentLen is the shorter of PostgreSQL's (63) and MySQL's (64) identifier
// limits; PostgreSQL silently truncates longer names, MySQL rejects them.
const maxIdentLen = 63

// mysqlTimeLayout renders a DATETIME(6) value. Timestamps cross the MySQL wire
// as UTC strings in this layout, never as time.Time: go-sql-driver/mysql
// converts a bound time.Time into its configured loc and parses DATETIME in
// that loc, which would shift every instant for a DSN with loc != UTC.
const mysqlTimeLayout = "2006-01-02 15:04:05.000000"

// dialect holds the per-flavor differences: placeholders, quoting, the database
// clock, interval arithmetic, and how timestamps cross the wire.
type dialect struct {
	name     Dialect
	quote    byte
	numbered bool   // $1, $2 … instead of ?
	now      string // the backend clock
}

func dialectFor(d Dialect) (dialect, error) {
	switch d {
	case DialectPostgres:
		// now() is the transaction start time, so a fetch transaction compares
		// and stamps against one instant — like MongoDB's $$NOW.
		return dialect{name: d, quote: '"', numbered: true, now: "now()"}, nil
	case DialectMySQL:
		return dialect{name: d, quote: '`', now: "UTC_TIMESTAMP(6)"}, nil
	default:
		return dialect{}, fmt.Errorf("%w: %q", ErrUnsupportedDialect, d)
	}
}

// table validates and quotes a possibly schema-qualified table name. Each part
// must fit the identifier limit: PostgreSQL would silently truncate a longer
// one, so two distinct configured names could address the same table.
func (d dialect) table(name string) (string, error) {
	if !identifier.MatchString(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidTableName, name)
	}
	parts := strings.Split(name, ".")
	for i, p := range parts {
		if len(p) > maxIdentLen {
			return "", fmt.Errorf("%w: %q exceeds %d characters", ErrInvalidTableName, p, maxIdentLen)
		}
		parts[i] = d.ident(p)
	}
	return strings.Join(parts, "."), nil
}

func (d dialect) ident(name string) string {
	return string(d.quote) + name + string(d.quote)
}

// indexName derives a quoted index name from the unqualified table name. A
// name over maxIdentLen keeps a readable prefix and ends in a hash of the table
// name, so truncation cannot make two tables' indexes collide.
func (d dialect) indexName(table, suffix string) string {
	name := table + "_" + suffix + "_idx"
	if len(name) > maxIdentLen {
		h := fnv.New32a()
		_, _ = h.Write([]byte(table))
		tail := fmt.Sprintf("_%08x_%s_idx", h.Sum32(), suffix)
		name = table[:maxIdentLen-len(tail)] + tail
	}
	return d.ident(name)
}

// bind rewrites the ? placeholders of a statement written by this package into
// the dialect's form. Statements authored here never contain ? in a literal.
func (d dialect) bind(query string) string {
	if !d.numbered {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 16) //nolint:mnd // room for multi-digit placeholders
	n := 1
	for i := range len(query) {
		if query[i] == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			n++
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
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

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// Dialect selects the SQL flavor the storage speaks.
type Dialect string

const (
	// DialectPostgres targets PostgreSQL 12 or newer.
	DialectPostgres Dialect = "postgres"
	// DialectMySQL targets MySQL 8.0+ and MariaDB 10.6+.
	DialectMySQL Dialect = "mysql"
)

// Sentinel errors returned by the storage.
var (
	// ErrUnsupportedDialect is returned by [New] for a dialect other than
	// [DialectPostgres] or [DialectMySQL].
	ErrUnsupportedDialect = sqldialect.ErrUnsupportedDialect
	// ErrInvalidTableName is returned by [New] when the table name is not a
	// plain SQL identifier, optionally qualified by one schema name, or a part
	// of it exceeds 63 characters.
	ErrInvalidTableName = sqldialect.ErrInvalidTableName
	// ErrValueTooLong is returned by Store and StoreBatch, before anything is
	// written, for an event ID longer than [MaxIDLength] characters or a
	// queryable field longer than [MaxFilterBytes] bytes. Rejecting them up
	// front matters: PostgreSQL silently trims excess trailing spaces from the
	// ID, an oversized value would exceed a B-tree index entry, and MySQL
	// without strict mode would truncate it, so a query by the original value
	// would miss the event.
	ErrValueTooLong = errors.New("sqldb: value exceeds its column length")
)

// MaxIDLength bounds an event ID, in characters.
const MaxIDLength = 255

// MaxFilterBytes bounds each queryable field — type, action, actor ID and
// type, resource type and ID, status, request and trace IDs — in bytes, so
// every index entry fits PostgreSQL's B-tree limit and MySQL stores the value
// whole.
const MaxFilterBytes = 255

// dialect holds the per-flavor differences: identifier quoting, placeholder
// style, and how strings are bound.
type dialect struct {
	sqldialect.Style
	name Dialect
}

func dialectFor(d Dialect) (dialect, error) {
	switch d {
	case DialectPostgres:
		return dialect{Style: sqldialect.Postgres, name: d}, nil
	case DialectMySQL:
		return dialect{Style: sqldialect.MySQL, name: d}, nil
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

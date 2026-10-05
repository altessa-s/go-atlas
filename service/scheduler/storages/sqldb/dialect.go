// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/mariadb"
	"github.com/altessa-s/go-atlas/data/filter/translators/postgres"
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
	// ErrInvalidTableName is returned by [New] when a table name is not a plain
	// SQL identifier, optionally qualified by one schema name, or a part of it
	// exceeds 63 characters.
	ErrInvalidTableName = sqldialect.ErrInvalidTableName
	// ErrValueTooLong is returned by writes whose ID-like value exceeds its
	// column: task, history and run IDs hold at most [MaxIDLength] characters,
	// a schedule at most [MaxScheduleLength]. Rejecting them up front matters
	// because PostgreSQL silently trims excess trailing spaces, which would
	// make "id" and "id " (past the limit) the same row.
	ErrValueTooLong = errors.New("sqldb: value exceeds its column length")
)

// Column limits for ID-like values, in characters.
const (
	// MaxIDLength bounds task, history and run IDs.
	MaxIDLength = 255
	// MaxScheduleLength bounds a task's schedule expression.
	MaxScheduleLength = 1024
)

// translator is the part of the data/filter SQL translators the storage uses.
type translator interface {
	Translate(node filter.Node) (string, []any, error)
}

// dialect holds the per-flavor differences: identifier quoting and placeholder
// style, and the matching data/filter translator.
type dialect struct {
	sqldialect.Style
	name          Dialect
	newTranslator func(opts ...filter.TranslatorOption) (translator, error)
}

func dialectFor(d Dialect) (dialect, error) {
	switch d {
	case DialectPostgres:
		return dialect{Style: sqldialect.Postgres, name: d, newTranslator: func(opts ...filter.TranslatorOption) (translator, error) {
			return postgres.NewTranslator(opts...)
		}}, nil
	case DialectMySQL:
		return dialect{Style: sqldialect.MySQL, name: d, newTranslator: func(opts ...filter.TranslatorOption) (translator, error) {
			return mariadb.NewTranslator(opts...)
		}}, nil
	default:
		return dialect{}, fmt.Errorf("%w: %q", ErrUnsupportedDialect, d)
	}
}

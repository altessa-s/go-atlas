// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/data/filter/translators/mariadb"
	"github.com/altessa-s/go-atlas/data/filter/translators/postgres"
)

// Dialect selects the SQL flavor the storage speaks.
type Dialect string

const (
	// DialectPostgres targets PostgreSQL 12 or newer.
	DialectPostgres Dialect = "postgres"
	// DialectMySQL targets MySQL 8.0.17+ and MariaDB 10.6+.
	DialectMySQL Dialect = "mysql"
)

// Sentinel errors returned by the storage.
var (
	// ErrUnsupportedDialect is returned by [New] for a dialect other than
	// [DialectPostgres] or [DialectMySQL].
	ErrUnsupportedDialect = errors.New("sqldb: unsupported dialect")
	// ErrInvalidTableName is returned by [New] when a table name is not a plain
	// SQL identifier, optionally qualified by one schema name.
	ErrInvalidTableName = errors.New("sqldb: invalid table name")
	// ErrUnsupportedVersion is returned by [Storage.EnsureSchema] when the
	// server is older than MySQL 8.0.17 or MariaDB 10.6.
	ErrUnsupportedVersion = errors.New("sqldb: unsupported database version")
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

// identifier matches a plain SQL identifier with an optional schema qualifier.
// Table names are the only SQL fragments that cannot be bound as parameters,
// so they are restricted to this shape and then quoted.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// translator is the part of the data/filter SQL translators the storage uses.
type translator interface {
	Translate(node filter.Node) (string, []any, error)
}

// dialect holds the per-flavor differences: placeholder style, identifier
// quoting, and the matching data/filter translator.
type dialect struct {
	name          Dialect
	quote         byte
	numbered      bool // $1, $2 … instead of ?
	newTranslator func(opts ...filter.TranslatorOption) (translator, error)
}

func dialectFor(d Dialect) (dialect, error) {
	switch d {
	case DialectPostgres:
		return dialect{name: d, quote: '"', numbered: true, newTranslator: func(opts ...filter.TranslatorOption) (translator, error) {
			return postgres.NewTranslator(opts...)
		}}, nil
	case DialectMySQL:
		return dialect{name: d, quote: '`', newTranslator: func(opts ...filter.TranslatorOption) (translator, error) {
			return mariadb.NewTranslator(opts...)
		}}, nil
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
		parts[i] = string(d.quote) + p + string(d.quote)
	}
	return strings.Join(parts, "."), nil
}

// bind rewrites the ? placeholders of a statement written by this package into
// the dialect's form, numbering from start. Only statements authored here pass
// through bind, and they never contain a ? inside a literal.
func (d dialect) bind(query string, start int) string {
	if !d.numbered {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8) //nolint:mnd // room for multi-digit placeholders
	n := start
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

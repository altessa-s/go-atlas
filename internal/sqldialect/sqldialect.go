// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas/internal/sqlident"
)

// Sentinel errors shared by the SQL storages, which re-export them so that
// errors.Is matches whichever package a caller checks against.
var (
	// ErrUnsupportedDialect reports a dialect a storage does not implement.
	ErrUnsupportedDialect = errors.New("sqldb: unsupported dialect")
	// ErrValueTooLong reports a value exceeding its column length.
	ErrValueTooLong = errors.New("sqldb: value exceeds its column length")
)

// Style is the identifier quoting and placeholder form of a SQL flavor.
type Style struct {
	// Quote is the identifier quote character.
	Quote byte
	// Numbered selects $1, $2 … placeholders instead of ?.
	Numbered bool
}

// Built-in styles.
var (
	// Postgres quotes with double quotes and numbers its placeholders.
	Postgres = Style{Quote: '"', Numbered: true}
	// MySQL quotes with backticks and uses positional ? placeholders; it
	// serves MariaDB too.
	MySQL = Style{Quote: '`'}
)

// Ident quotes one identifier part. The caller guarantees it is a plain
// identifier.
func (s Style) Ident(name string) string {
	return string(s.Quote) + name + string(s.Quote)
}

// Table validates and quotes a possibly schema-qualified table name: a plain
// identifier ([A-Za-z_][A-Za-z0-9_]*), optionally qualified by one schema
// name. Table and column names are the only SQL fragments that cannot be
// bound as parameters, so they are restricted to this shape and then quoted.
// Each part must fit [sqlident.MaxIdentLen]; a name of any other shape is rejected
// with [sqlident.ErrInvalidTableName].
//
// It sits on the per-request path of the SQL translators (one call per
// selected or sorted column), so it validates by hand and allocates only the
// result.
func (s Style) Table(name string) (string, error) {
	if err := sqlident.Validate(name); err != nil {
		return "", err
	}
	schema, table, qualified := strings.Cut(name, ".")

	var b strings.Builder
	b.Grow(len(name) + 4) //nolint:mnd // two quotes per part, at most two parts
	b.WriteByte(s.Quote)
	b.WriteString(schema)
	b.WriteByte(s.Quote)
	if qualified {
		b.WriteByte('.')
		b.WriteByte(s.Quote)
		b.WriteString(table)
		b.WriteByte(s.Quote)
	}
	return b.String(), nil
}

// IndexName derives a quoted index name from an unqualified table name, so
// storages over different tables never collide on index names. A name that
// would exceed [sqlident.MaxIdentLen] keeps a readable prefix and ends in an FNV-32a
// hash of the full table name, so truncation cannot make two tables' indexes
// collide. If the suffix itself exceeds the available space, the hash covers
// the full derived name and the readable prefix includes the table and suffix.
func (s Style) IndexName(table, suffix string) string {
	name := table + "_" + suffix + "_idx"
	if len(name) > sqlident.MaxIdentLen {
		h := fnv.New32a()
		_, _ = h.Write([]byte(table))
		tail := fmt.Sprintf("_%08x_%s_idx", h.Sum32(), suffix)
		if len(tail) > sqlident.MaxIdentLen {
			// Preserve existing index names for ordinary suffixes. When the
			// suffix itself is too long, hash the full name before truncating.
			h.Reset()
			_, _ = h.Write([]byte(name))
			tail = fmt.Sprintf("_%08x_idx", h.Sum32())
			name = name[:sqlident.MaxIdentLen-len(tail)] + tail
		} else {
			name = table[:sqlident.MaxIdentLen-len(tail)] + tail
		}
	}
	return s.Ident(name)
}

// Bind rewrites the ? placeholders of a statement into the style's form,
// numbering from start; a positional style returns the query unchanged. Only
// statements authored by the storages pass through Bind, and they never
// contain a ? inside a literal.
func (s Style) Bind(query string, start int) string {
	if !s.Numbered {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 16) //nolint:mnd // room for multi-digit placeholders
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

// Unqualified returns the table part of a possibly schema-qualified name.
func Unqualified(name string) string {
	if _, table, ok := strings.CutLast(name, "."); ok {
		return table
	}
	return name
}

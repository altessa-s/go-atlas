// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect

import (
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
)

// Sentinel errors shared by the SQL storages, which re-export them so that
// errors.Is matches whichever package a caller checks against.
var (
	// ErrUnsupportedDialect reports a dialect a storage does not implement.
	ErrUnsupportedDialect = errors.New("sqldb: unsupported dialect")
	// ErrInvalidTableName reports a table name that is not a plain SQL
	// identifier, optionally qualified by one schema name, or whose parts
	// exceed [MaxIdentLen].
	ErrInvalidTableName = errors.New("sqldb: invalid table name")
)

// MaxIdentLen is the shorter of PostgreSQL's (63) and MySQL's (64) identifier
// limits. PostgreSQL silently truncates longer names, so two distinct
// configured names could address the same table or index; MySQL rejects them.
const MaxIdentLen = 63

// identifier matches a plain SQL identifier with an optional schema qualifier.
// Table names are the only SQL fragments that cannot be bound as parameters,
// so they are restricted to this shape and then quoted.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

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

// Table validates and quotes a possibly schema-qualified table name. Each part
// must fit [MaxIdentLen]; a name of any other shape is rejected with
// [ErrInvalidTableName].
func (s Style) Table(name string) (string, error) {
	if !identifier.MatchString(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidTableName, name)
	}
	parts := strings.Split(name, ".")
	for i, p := range parts {
		if len(p) > MaxIdentLen {
			return "", fmt.Errorf("%w: %q exceeds %d characters", ErrInvalidTableName, p, MaxIdentLen)
		}
		parts[i] = s.Ident(p)
	}
	return strings.Join(parts, "."), nil
}

// IndexName derives a quoted index name from an unqualified table name, so
// storages over different tables never collide on index names. A name that
// would exceed [MaxIdentLen] keeps a readable prefix and ends in an FNV-32a
// hash of the full table name, so truncation cannot make two tables' indexes
// collide.
func (s Style) IndexName(table, suffix string) string {
	name := table + "_" + suffix + "_idx"
	if len(name) > MaxIdentLen {
		h := fnv.New32a()
		_, _ = h.Write([]byte(table))
		tail := fmt.Sprintf("_%08x_%s_idx", h.Sum32(), suffix)
		name = table[:MaxIdentLen-len(tail)] + tail
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
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// Qualifier returns the schema part of a schema-qualified name, or "" for an
// unqualified one.
func Qualifier(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[:i]
	}
	return ""
}

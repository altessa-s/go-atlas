// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlident

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidTableName reports a table name that is not a plain SQL
// identifier, optionally qualified by one schema name, or whose parts
// exceed [MaxIdentLen].
var ErrInvalidTableName = errors.New("sqldb: invalid table name")

// MaxIdentLen is the shorter of PostgreSQL's (63) and MySQL's (64) identifier
// limits. PostgreSQL silently truncates longer names, so two distinct
// configured names could address the same table or index; MySQL rejects them.
const MaxIdentLen = 63

// Validate checks a plain SQL identifier optionally qualified by one schema name.
// Each part must match [A-Za-z_][A-Za-z0-9_]* and fit within [MaxIdentLen].
func Validate(name string) error {
	schema, table, qualified := strings.Cut(name, ".")
	if !isIdent(schema) || (qualified && !isIdent(table)) {
		return fmt.Errorf("%w: %q", ErrInvalidTableName, name)
	}
	for _, p := range [...]string{schema, table} {
		if len(p) > MaxIdentLen {
			return fmt.Errorf("%w: %q exceeds %d characters", ErrInvalidTableName, p, MaxIdentLen)
		}
	}
	return nil
}

// isIdent reports whether p matches [A-Za-z_][A-Za-z0-9_]*.
func isIdent(p string) bool {
	if p == "" {
		return false
	}
	for i := range len(p) {
		switch c := p[i]; {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

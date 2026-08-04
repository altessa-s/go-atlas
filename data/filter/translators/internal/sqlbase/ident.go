// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlbase

import (
	"strings"

	"github.com/altessa-s/go-atlas/data/filter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ValidateIdent rejects any column name that is not a dot-separated
// sequence of plain SQL identifiers ([A-Za-z_][A-Za-z0-9_]*).
//
// The check is deliberately stricter than quoting alone requires. The
// column position is the one part of a generated clause that no
// placeholder can cover, so it must not be reachable by anything a
// [filter.WithFieldMapping] entry could smuggle in — an expression, a
// subscript, a quote, a comment.
func ValidateIdent(name string) error {
	if name == "" {
		return coreerrs.Wrap(filter.ErrInvalidExpression, "empty column name")
	}
	for segment := range strings.SplitSeq(name, ".") {
		if !isIdentSegment(segment) {
			return coreerrs.Wrapf(filter.ErrInvalidExpression, "%q is not a valid SQL column name", name)
		}
	}
	return nil
}

// QuoteWhole validates name and wraps it in quote as a single
// identifier, dots included. This is the ClickHouse policy: a Nested
// column is literally stored under the name "address.city".
func QuoteWhole(name string, quote byte) (string, error) {
	if err := ValidateIdent(name); err != nil {
		return "", err
	}

	var b strings.Builder
	b.Grow(len(name) + 2) //nolint:mnd // opening + closing quote.
	b.WriteByte(quote)
	b.WriteString(name)
	b.WriteByte(quote)
	return b.String(), nil
}

// QuoteQualified validates name and quotes each dot-separated segment
// on its own, producing the standard SQL qualified form
// ("table"."column"). This is the MariaDB and PostgreSQL policy.
func QuoteQualified(name string, quote byte) (string, error) {
	if err := ValidateIdent(name); err != nil {
		return "", err
	}

	var b strings.Builder
	b.Grow(len(name) + 2*(strings.Count(name, ".")+1)) //nolint:mnd // two quotes per segment.
	for i, segment := range strings.Split(name, ".") {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteByte(quote)
		b.WriteString(segment)
		b.WriteByte(quote)
	}
	return b.String(), nil
}

// isIdentSegment reports whether s matches [A-Za-z_][A-Za-z0-9_]*.
//
// Quoting characters need no escaping downstream precisely because they
// cannot survive this check: a name containing the dialect's quote byte
// is rejected before it reaches a quoter.
func isIdentSegment(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

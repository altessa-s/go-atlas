// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlbase

import "github.com/altessa-s/go-atlas/data/filter"

// ValueFunc renders a Go value as SQL text in the translator's active
// mode — a bind placeholder with the value collected into the argument
// slice, or an inline literal.
//
// A [Dialect] receives one when rendering a string predicate so it can
// bind the same operand more than once: `endsWith` has no native form in
// MariaDB or PostgreSQL and compiles to `right(col, length(x)) = x`,
// where x appears twice and must therefore be bound twice.
type ValueFunc func(v any) (string, error)

// Dialect supplies the per-backend rendering decisions the shared walker
// cannot make on its own. Everything else — the AST walk, the allow-list
// and field-type checks, depth accounting, argument collection, IN lists,
// null handling and the comparison operators — is identical across the
// SQL backends and lives in [Translator].
//
// Implementations are stateless value types; the walker holds exactly one
// for its lifetime.
type Dialect interface {
	// QuoteIdent renders an already-mapped field name as a quoted column
	// reference, rejecting anything that is not a plain identifier with
	// [filter.ErrInvalidExpression]. The dot policy belongs here: a
	// dotted name is a qualified `"table"."column"` in MariaDB and
	// PostgreSQL but a single Nested column in ClickHouse.
	QuoteIdent(name string) (string, error)

	// Placeholder renders the bind marker for the n-th argument, counting
	// from 1. Positional dialects ignore n; PostgreSQL renders "$n".
	Placeholder(n int) string

	// FormatLiteral renders a Go value as SQL text for inline mode.
	// Values the dialect cannot express — a NaN in MariaDB, say — must
	// return [filter.ErrUnsupportedType] rather than an approximation.
	FormatLiteral(v any) (string, error)

	// SizeExpr renders size() over an already-quoted column.
	SizeExpr(col string) string

	// StringPredicate renders contains, startsWith, endsWith or matches
	// over an already-quoted column. The needle arrives raw; the dialect
	// passes it through value to obtain its SQL text, once per occurrence.
	// The regex length cap is already enforced for [filter.OpMatches].
	StringPredicate(op filter.Operator, col, needle string, value ValueFunc) (string, error)
}

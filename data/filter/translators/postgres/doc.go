// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package postgres provides a translator that converts filter AST nodes to PostgreSQL WHERE clauses.
//
// The translator maps the CEL operations onto PostgreSQL operators and built-ins — comparisons, logical
// operators, IN lists, the string predicates, length() and null checks. Operations without a PostgreSQL
// counterpart return filter.ErrUnsupportedOperation.
//
// # Basic Usage
//
//	parser, _ := filter.NewParser()
//	ast, _ := parser.Parse(ctx, `status == 2 && type in [1, 2]`)
//
//	trans, err := postgres.NewTranslator()
//	if err != nil {
//	    return err
//	}
//	where, args, err := trans.Translate(ast)
//	// where: ("status" = $1) AND ("type" IN ($2, $3))
//	// args:  []any{int64(2), int64(1), int64(2)}
//
//	rows, err := conn.Query(ctx, "SELECT * FROM events WHERE "+where, args...)
//
// Placeholders are numbered, counting up in the order the arguments are collected. A clause therefore
// cannot be spliced into a query that already binds parameters without renumbering — build the whole
// WHERE from one Translate call, or append the returned args after your own and shift accordingly.
//
// # Output Modes
//
// Translate is the parameterized form and the one to reach for by default: literals never enter the SQL
// text, so a filter built from request data cannot alter the shape of the query. TranslateInline renders
// the same clause with literals in place, for the cases a placeholder cannot serve — view definitions,
// generated DDL, logs, debugging.
//
// A nil AST translates to a match-all predicate ("1 = 1") rather than an empty string, so the
// `"... WHERE " + where` concatenation above stays valid without a special case at the call site.
//
// # Supported Operations
//
//	Comparison: ==, !=, <, >, <=, >=            → =, !=, <, >, <=, >=
//	Logical:    &&, ||, !                       → AND, OR, NOT
//	Membership: in                              → IN (...)
//	Existence:  has(), field == null            → IS NOT NULL, IS NULL
//	String:     contains()                      → strpos(col, $n) > 0
//	            startsWith()                    → starts_with(col, $n)
//	            endsWith()                      → right(col, length($n)) = $n+1
//	            matches()                       → col ~ $n
//	Size:       size()                          → length(col), inside a comparison only
//
// substring() is rejected, as it is by every other translator. size() used on its own — outside a
// comparison — is rejected too: length() is an integer expression, and there is no predicate to test.
//
// PostgreSQL has no endsWith function, so that predicate compiles to a suffix comparison and binds its
// operand twice. One CEL argument therefore consumes two placeholders; the argument slice reflects that.
// starts_with() requires PostgreSQL 11 or newer.
//
// # Column Names
//
// Field names are checked against the allow-list, run through the field mapping, validated as plain SQL
// identifiers, and emitted double-quoted per dot-separated segment — "address"."city". Anything else — an
// expression, a subscript, a quote, a comment — is rejected with filter.ErrInvalidExpression: the column
// position is the one part of the generated SQL that no placeholder can cover, so it is held to a
// stricter standard than quoting alone would require. To filter on a jsonb path or a computed value, map
// the CEL name onto a generated column.
//
// Quoting also pins the case. PostgreSQL folds unquoted identifiers to lower case, so an unquoted
// createdAt would resolve to the column createdat; quoted, it means exactly createdAt. Where the table
// uses snake_case, say so with filter.WithFieldMapping rather than relying on folding.
//
// # Types
//
// PostgreSQL is stricter about types than the other backends this package family targets. A bare
// identifier used as a condition compiles to `col = TRUE` and requires an actual boolean column — there is
// no integer-to-boolean coercion. size() compiles to length(), which covers text; array and jsonb columns
// need cardinality() and jsonb_array_length() instead, so map those fields onto a generated column if you
// need to filter on their size.
//
// # Null Semantics
//
// A SQL row always carries every column of its table, so there is no "missing field" for has() to detect;
// it compiles to IS NOT NULL and is therefore meaningful only against a nullable column. The same caveat
// applies to `!=` under three-valued logic: a NULL row does not satisfy `col != 'x'`, since the
// comparison yields NULL rather than true. Write `col != "x" || col == null` where the NULL row should
// match.
//
// # Regular Expressions
//
// matches() compiles to the case-sensitive POSIX operator `~`. PostgreSQL's engine backtracks, so a
// crafted pattern can be made expensive server-side. filter.WithMaxRegexLength bounds the blast radius but
// does not eliminate it — expose matches() to trusted callers, or tighten the cap. A statement_timeout is
// the reliable backstop.
//
// # Timestamps
//
// timestamp(...) literals bind as time.Time in the parameterized form and render as a UTC
// TIMESTAMP WITH TIME ZONE literal at microsecond precision inline.
//
// # Inline Rendering
//
// TranslateInline emits escape-string literals, E'...'. The prefix is what makes one escaping rule
// correct on any server: inside a plain '...' literal the backslash is an escape character only when
// standard_conforming_strings is off, so no plain-literal escaping is correct in both settings.
//
// # Concurrency
//
// A Translator accumulates per-call state and is not safe for concurrent use. Construct one per goroutine,
// or guard it with a mutex.
package postgres

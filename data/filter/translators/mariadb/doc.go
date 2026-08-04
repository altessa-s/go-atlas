// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mariadb provides a translator that converts filter AST nodes to MariaDB SQL WHERE clauses.
//
// The translator maps the CEL operations onto MariaDB operators and built-ins — comparisons, logical
// operators, IN lists, the string predicates, CHAR_LENGTH and null checks. Operations without a MariaDB
// counterpart return filter.ErrUnsupportedOperation. The output is equally valid MySQL.
//
// # Basic Usage
//
//	parser, _ := filter.NewParser()
//	ast, _ := parser.Parse(ctx, `status == 2 && type in [1, 2]`)
//
//	trans, err := mariadb.NewTranslator()
//	if err != nil {
//	    return err
//	}
//	where, args, err := trans.Translate(ast)
//	// where: (`status` = ?) AND (`type` IN (?, ?))
//	// args:  []any{int64(2), int64(1), int64(2)}
//
//	rows, err := db.QueryContext(ctx, "SELECT * FROM events WHERE "+where, args...)
//
// # Output Modes
//
// Translate is the parameterized form and the one to reach for by default: literals never enter the SQL
// text, so a filter built from request data cannot alter the shape of the query. TranslateInline renders
// the same clause with literals in place, for the cases a placeholder cannot serve — view definitions,
// generated DDL, logs, debugging. See the caveat under Inline Rendering.
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
//	String:     contains()                      → LOCATE(?, col) > 0
//	            startsWith()                    → LOCATE(?, col) = 1
//	            endsWith()                      → RIGHT(col, CHAR_LENGTH(?)) = ?
//	            matches()                       → col REGEXP ?
//	Size:       size()                          → CHAR_LENGTH(col), inside a comparison only
//
// substring() is rejected, as it is by every other translator. size() used on its own — outside a
// comparison — is rejected too: CHAR_LENGTH is an integer expression, and there is no predicate to test.
//
// MariaDB has no endsWith function, so that predicate compiles to a suffix comparison and binds its
// operand twice. One CEL argument therefore consumes two placeholders; the argument slice reflects that.
//
// # Column Names
//
// Field names are checked against the allow-list, run through the field mapping, validated as plain SQL
// identifiers, and emitted backtick-quoted per dot-separated segment — `address`.`city`. Anything else —
// an expression, a subscript, a quote, a comment — is rejected with filter.ErrInvalidExpression: the
// column position is the one part of the generated SQL that no placeholder can cover, so it is held to a
// stricter standard than quoting alone would require. To filter on a JSON path or a computed value, map
// the CEL name onto a generated column.
//
// # Collation
//
// String comparison follows the column's collation, and MariaDB's defaults (utf8mb4_general_ci and
// friends) are case-insensitive. Every string predicate here inherits that: contains, startsWith,
// endsWith, REGEXP and plain equality all match case-insensitively unless the column or the connection
// says otherwise. This is a real semantic difference from the ClickHouse and PostgreSQL translators,
// which are case-sensitive. Use an explicit _bin or _cs collation where the distinction matters.
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
// matches() compiles to REGEXP, which MariaDB 10.0.5+ evaluates with PCRE. Unlike RE2 it can backtrack,
// so a crafted pattern can be made expensive server-side. filter.WithMaxRegexLength bounds the blast
// radius but does not eliminate it — expose matches() to trusted callers, or tighten the cap.
//
// # Timestamps
//
// timestamp(...) literals bind as time.Time in the parameterized form and render as a UTC DATETIME
// literal at microsecond precision inline. Store the corresponding columns in UTC.
//
// # Inline Rendering
//
// TranslateInline doubles the quote in a string literal, which closes the break-out in every sql_mode. It
// also doubles the backslash, which is correct under the default mode where the backslash is an escape
// character. Under NO_BACKSLASH_ESCAPES it is not, and a value containing a backslash comes out with it
// doubled — a fidelity bug, never an injection. Use the parameterized form where exact values matter.
//
// # Concurrency
//
// A Translator accumulates per-call state and is not safe for concurrent use. Construct one per goroutine,
// or guard it with a mutex.
package mariadb

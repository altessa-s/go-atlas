// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package clickhouse provides a translator that converts filter AST nodes to ClickHouse SQL WHERE clauses.
//
// The translator implements the filter.Visitor interface and maps the CEL operations onto native ClickHouse
// operators and functions — comparisons, logical operators, IN lists, the string predicates, length() and
// null checks. Operations without a ClickHouse counterpart return filter.ErrUnsupportedOperation.
//
// # Basic Usage
//
//	parser, _ := filter.NewParser()
//	ast, _ := parser.Parse(ctx, `status == 2 && type in [1, 2]`)
//
//	trans, err := clickhouse.NewTranslator()
//	if err != nil {
//	    return err
//	}
//	where, args, err := trans.Translate(ast)
//	// where: (`status` = ?) AND (`type` IN (?, ?))
//	// args:  []any{int64(2), int64(1), int64(2)}
//
//	rows, err := conn.Query(ctx, "SELECT * FROM events WHERE "+where, args...)
//
// # Output Modes
//
// Translate is the parameterized form and the one to reach for by default: literals never enter the SQL
// text, so a filter built from request data cannot alter the shape of the query. TranslateInline renders
// the same clause with literals in place, for the cases a placeholder cannot serve — materialized view
// definitions, generated DDL, logs, debugging.
//
// A nil AST translates to a match-all predicate ("1 = 1") rather than an empty string, so the
// `"... WHERE " + where` concatenation above stays valid without a special case at the call site.
//
// # With Options
//
//	trans, err := clickhouse.NewTranslator(
//	    filter.WithUntrustedInput(),
//	    filter.WithAllowedFields("status", "createdAt", "organizationIds"),
//	    filter.WithFieldMapping(map[string]string{
//	        "organizationIds": "organization_id",
//	        "createdAt":       "created_at",
//	    }),
//	)
//
// # Supported Operations
//
//	Comparison: ==, !=, <, >, <=, >=            → =, !=, <, >, <=, >=
//	Logical:    &&, ||, !                       → AND, OR, NOT
//	Membership: in                              → IN (...)
//	Existence:  has(), field == null            → IS NOT NULL, IS NULL
//	String:     contains()                      → position(col, ?) > 0
//	            startsWith(), endsWith()        → startsWith(col, ?), endsWith(col, ?)
//	            matches()                       → match(col, ?)
//	Size:       size()                          → length(col), inside a comparison only
//
// substring() is rejected, as it is by every other translator. size() used on its own — outside a
// comparison — is rejected too: length() is an integer expression, and there is no predicate to test.
//
// # Column Names
//
// Field names are checked against the allow-list, run through the field mapping, validated as plain
// ClickHouse identifiers, and emitted backtick-quoted. Dots survive as part of a single quoted name
// ("address.city"), which is how Nested columns are stored. Anything else — an expression, a Map
// subscript, a quote — is rejected with filter.ErrInvalidExpression: the column position is the one part
// of the generated SQL that no placeholder can cover, so it is held to a stricter standard than quoting
// alone would require. To filter on a Map key or a computed value, map the CEL name onto a materialized
// column.
//
// # Null Semantics
//
// ClickHouse rows always carry every column of the table, so there is no "missing field" for has() to
// detect; it compiles to IS NOT NULL and is therefore meaningful only against a Nullable column. The same
// caveat applies to `!=` under three-valued logic: a NULL row does not satisfy `col != 'x'`, since the
// comparison yields NULL rather than true. Write `col != 'x' || col == null` where the NULL row should
// match.
//
// # Timestamps
//
// timestamp(...) literals bind as time.Time in the parameterized form and render as
// toDateTime64('...', 3, 'UTC') inline. Inline rendering is millisecond-precision and drops anything
// finer; the parameterized form leaves precision to the driver.
//
// # Concurrency
//
// A Translator accumulates per-call state and is not safe for concurrent use. Construct one per goroutine,
// or guard it with a mutex.
package clickhouse

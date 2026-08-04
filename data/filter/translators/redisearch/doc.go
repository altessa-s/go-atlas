// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redisearch provides a translator that converts filter AST nodes to RediSearch query strings.
//
// The translator implements the filter.Visitor interface and supports standard CEL
// operations including comparisons, logical operators, membership tests, and string functions
// (contains, startsWith). A schema mapping is required to generate correct syntax for
// NUMERIC, TAG, and TEXT field types.
//
// # Basic Usage
//
//	parser, _ := filter.NewParser()
//	ast, _ := parser.Parse(ctx, `status == 1 && priority >= 3`)
//
//	schema := map[string]redisearch.FieldType{
//	    "status":   redisearch.FieldTypeNumeric,
//	    "priority": redisearch.FieldTypeNumeric,
//	}
//	trans, err := redisearch.NewTranslator(schema)
//	if err != nil {
//	    return err
//	}
//	query, _ := trans.Translate(ast)
//	// Result: "(@status:[1 1] @priority:[3 +inf])"
//
// # With Options
//
//	trans, err := redisearch.NewTranslator(schema,
//	    filter.WithAllowedFields("status", "priority", "id"),
//	    filter.WithFieldMapping(map[string]string{
//	        "lastRunAt": "lastRunAt",
//	    }),
//	    filter.WithMaxDepth(10),
//	)
//
// # Supported Operations
//
// Comparison: ==, !=, <, >, <=, >= (NUMERIC fields use range syntax, TAG fields use tag syntax)
// Logical: &&, ||, !
// Membership: in (rendered per field type — see below)
// String: contains(), startsWith() (TEXT fields only)
//
// The schema is not advisory. A field's type decides the shape of every query built against it, and
// `in` follows it too: a union of exact ranges for NUMERIC, a tag set for TAG, a term union for TEXT.
// A tag set against a NUMERIC field would match nothing at all — silently, which is worse than failing.
//
// A bare identifier used as a condition becomes a boolean TAG test — `active` translates to
// @active:{true}, `!active` to -@active:{true} — at the root of an expression and on either side of a
// logical operator. The TAG form is fixed rather than resolved through the schema: a boolean is only
// ever indexed as a TAG.
//
// # Unsupported Operations
//
// has(), matches(), endsWith(), size() return ErrUnsupportedOperation.
package redisearch

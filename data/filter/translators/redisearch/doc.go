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
//	trans := redisearch.NewTranslator(schema)
//	query, _ := trans.Translate(ast)
//	// Result: "(@status:[1 1] @priority:[3 +inf])"
//
// # With Options
//
//	trans := redisearch.NewTranslator(schema,
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
// Membership: in (TAG fields only)
// String: contains(), startsWith() (TEXT fields only)
//
// # Unsupported Operations
//
// has(), matches(), endsWith(), size() return ErrUnsupportedOperation.
package redisearch

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package postgres translates an [orderby.Spec] into the body of a PostgreSQL
// ORDER BY clause, the companion of data/filter/translators/postgres.
//
// The result lists the keys in order, each a quoted column and ASC or DESC,
// without the ORDER BY keywords: an empty [orderby.Spec] translates to "" so
// the caller can omit the clause.
//
// # Basic Usage
//
//	parser, _ := orderby.NewParser()
//	ob, _ := parser.Parse(ctx, "create_time desc, slug")
//
//	trans, err := postgres.NewTranslator(
//	    orderby.WithFieldMapping(map[string]string{"create_time": "created_at"}),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	clause, _ := trans.Translate(ob)
//	// clause == `"created_at" DESC, "slug" ASC`
//
// # Identifiers
//
// A sort column is the one part of a query no placeholder can cover, so every
// key, after field mapping, must be a plain column name or table.column
// ([A-Za-z_][A-Za-z0-9_]* per part, at most 63 characters each); each part is
// quoted on its own. Anything else — an expression, a JSON path, a quote — is
// rejected with [orderby.ErrInvalidFieldPath]. Map a nested field onto a
// generated column to sort by it.
//
// # NULL ordering
//
// No NULLS FIRST/LAST clause is emitted, so NULL placement is the database
// default: PostgreSQL sorts NULL as larger than any value: last in ASC, first in DESC.
// Add a unique column as the last key for a deterministic order.
//
// # Errors
//
// Returns [orderby.ErrFieldNotAllowed] when an entry references a field absent
// from the configured allow-list, [orderby.ErrInvalidFieldPath] for a key that
// does not map to a column name, and [orderby.ErrAllowlistRequired] when
// [orderby.WithUntrustedInput] was configured without
// [orderby.WithAllowedFields].
package postgres

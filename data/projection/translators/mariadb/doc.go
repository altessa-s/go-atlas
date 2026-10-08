// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mariadb translates a [projection.Spec] into the column list of a
// MariaDB/MySQL SELECT, the companion of data/orderby/translators/mariadb.
//
// # Basic Usage
//
//	parser, _ := projection.NewParser()
//	spec, _ := parser.Parse(ctx, "name")
//
//	trans, err := mariadb.NewTranslator(
//	    projection.WithAllowedFields("name", "email"),
//	    projection.WithRequiredFields("id"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	cols, _ := trans.Translate(spec)
//	// cols == ``id`, `name``
//
// # Identifiers
//
// A selected column is the one part of a query no placeholder can cover, so
// every storage name, after field mapping, must be a plain column name or
// table.column ([A-Za-z_][A-Za-z0-9_]* per part, at most 63 characters
// each); each part is quoted on its own. Anything else — an expression, a
// JSON path, a quote — is rejected with [projection.ErrInvalidFieldPath].
// Map a nested API field onto a column to select it.
//
// # Result shape
//
// Columns are emitted in lexical order of their storage names, without
// aliases; scan rows by column name. An empty request resolves to the
// default selection — the policy's default fields or allow-list roots. When
// the policy has neither, every column is selected and Translate returns ""
// so the caller writes `*`.
//
// # Errors
//
// Returns [projection.ErrFieldNotAllowed] for a path outside the allow-list
// or overlapping a denied field, [projection.ErrInvalidFieldPath] for a path
// that does not map to a column name, and from NewTranslator
// [projection.ErrAllowlistRequired], [projection.ErrConflictingPolicy] and
// [projection.ErrDefaultFieldsRequired].
package mariadb

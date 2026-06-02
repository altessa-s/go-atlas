// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package orderby parses the AIP-132 order_by DSL into a flat AST and translates
// it to database-specific sort representations.
//
// The DSL is the free-form string carried by List*Request.order_by on
// resource-oriented APIs (https://google.aip.dev/132):
//
//	order_by  := key ("," key)*
//	key       := field_path direction?
//	field_path:= ident ("." ident)*
//	direction := "asc" | "desc"
//
// Whitespace around commas and between the field and direction tokens is
// insignificant. The default direction is ascending. Empty or whitespace-only
// input parses to an empty [Spec] with no error, matching the AIP semantics
// of "no ordering specified". Case-sensitivity, the maximum number of keys, and
// the field allow-list are server concerns and are controlled via parser and
// translator options.
//
// # Basic Usage
//
//	parser, err := orderby.NewParser()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	ob, err := parser.Parse(ctx, "create_time desc, slug")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	trans, err := mongo.NewTranslator()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	sort, _ := trans.Translate(ob)
//	// Result: bson.D{{"create_time", -1}, {"slug", 1}}
//
// # Security
//
// The parser and translators ship with the same defense-in-depth controls as
// data/filter:
//
//   - WithMaxExpressionLength caps the raw input size.
//   - WithMaxKeys caps the number of sort keys.
//   - WithMaxFieldPathDepth caps the number of dotted segments in a single key.
//   - WithMaxFieldNameLength caps the total length of a single field path.
//   - WithAllowedFields restricts which fields a client may sort on. Entries
//     ending in ".*" allow a whole subtree; a bare "*" matches anything.
//   - WithFieldMapping (exact) and WithFieldPrefixMapping (subtree) map DSL
//     names to DB column names.
//   - WithUntrustedInput pairs with a non-empty allow-list to make "no
//     allow-list" a hard error rather than a permissive default.
//
// Array-index segments (tags.0, items.5.name) are rejected by default for
// strict AIP-132 conformance. Opt in with WithAllowArrayIndexPaths.
//
// # Architecture
//
// The DSL has no nesting and no operators, so the AST is a flat [Spec.Keys]
// slice rather than a visitor-style tree. Each translator iterates over the
// keys, applies the configured field allow-list and mapping, and emits the
// shape its storage backend expects.
package orderby

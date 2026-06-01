// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package meili provides a translator that converts filter AST nodes to Meilisearch filter expressions.
//
// The translator implements the filter.Visitor interface and supports the CEL operations expressible
// in Meilisearch's filter grammar — comparisons, logical operators, list membership, contains/startsWith
// predicates and field-existence checks. Operations without a Meilisearch counterpart return
// filter.ErrUnsupportedOperation.
//
// # Basic Usage
//
//	parser, _ := filter.NewParser()
//	ast, _ := parser.Parse(ctx, `status == 2 && type in [1, 2]`)
//
//	trans, err := meili.NewTranslator()
//	if err != nil {
//	    return err
//	}
//	expr, _ := trans.Translate(ast)
//	// Result: (status = 2) AND (type IN [1, 2])
//
// # With Options
//
//	trans, err := meili.NewTranslator(
//	    filter.WithUntrustedInput(),
//	    filter.WithAllowedFields("status", "type", "organizationIds"),
//	    filter.WithFieldMapping(map[string]string{
//	        "organizationIds": "organization_id",
//	    }),
//	)
//
// # Supported Operations
//
// Comparison: ==, !=, <, >, <=, >=
// Logical: &&, ||, !
// Membership: in, has()
// String: contains(), startsWith()
//
// # Timestamps
//
// timestamp(...) literals are emitted as Unix seconds — Meilisearch filters
// numeric attributes only, so the corresponding fields must be stored as
// numeric epoch seconds and listed in the index's filterableAttributes.
// Sub-second precision is dropped.
package meili

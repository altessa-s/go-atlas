// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redisearch translates an [orderby.Spec] into a single-field
// RediSearch sort instruction.
//
// RediSearch's FT.SEARCH command exposes ordering via a single
// `SORTBY <field> ASC|DESC` clause and does not support multi-field
// sorting. Inputs with more than one key are therefore rejected with
// [orderby.ErrTooManySortKeys] — callers must either drop the extra keys
// upstream or pick a different storage backend.
//
// # Basic Usage
//
//	parser, _ := orderby.NewParser()
//	ob, _ := parser.Parse(ctx, "createdAt desc")
//
//	trans, err := redisearch.NewTranslator()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	sortBy, _ := trans.Translate(ob)
//	// sortBy == redisearch.SortBy{Field: "createdAt", Descending: true}
//
// An empty Spec translates to a zero-value SortBy so callers can
// decide whether to append the SORTBY clause at all.
package redisearch

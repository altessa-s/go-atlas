// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package meili translates an [orderby.Spec] into a Meilisearch sort
// argument.
//
// Meilisearch accepts sort instructions as a list of "<field>:<direction>"
// strings (https://www.meilisearch.com/docs/reference/api/search#sort).
// This translator emits exactly that shape: a `[]string` whose entries are
// `"field:asc"` or `"field:desc"`, in the same order as the source
// order_by string.
//
// # Basic Usage
//
//	parser, _ := orderby.NewParser()
//	ob, _ := parser.Parse(ctx, "createdAt desc, name")
//
//	trans, err := meili.NewTranslator()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	sort, _ := trans.Translate(ob)
//	// sort == []string{"createdAt:desc", "name:asc"}
//
// An empty Spec translates to a nil slice with no error so callers can
// skip Meilisearch's `sort` parameter entirely.
package meili

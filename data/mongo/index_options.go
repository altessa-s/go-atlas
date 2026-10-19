// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=indexOptions --output=index_options_gen.go --option-type=IndexOption

// indexOptions configures [EnsureIndexes]. All fields are populated by generated [IndexOption] setters in index_options_gen.go.
type indexOptions struct {
	// dropOnSpecMismatch drops an existing index whose name matches a model but whose keys or options differ, and creates it with the
	// model's specification.
	dropOnSpecMismatch bool
	// dropObsolete names indexes to drop when present, before the models are created.
	dropObsolete []string `optgen:"append" optval:"notrim"`
}

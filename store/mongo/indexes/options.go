// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package indexes

//go:generate go tool optgen generate

// options configures [Ensure]. All fields are populated by generated [Option] setters in options_gen.go.
type options struct {
	// dropOnSpecMismatch drops an existing index whose name matches a model but whose keys or options differ, and creates it with the
	// model's specification.
	dropOnSpecMismatch bool
	// dropObsolete names indexes to drop when present, before the models are created.
	dropObsolete []string `optgen:"append" optval:"notrim"`
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json

//go:generate go tool optgen generate

// DefaultEscapeHTML is the default HTML-escaping mode of a [Codec].
const DefaultEscapeHTML = true

// options configures a [Codec].
type options struct {
	// indent ([WithIndent]) enables pretty-printing with 2-space indentation.
	indent bool `optval:"param"`
	// escapeHTML ([WithEscapeHTML]) controls HTML character escaping.
	escapeHTML bool `optgen:"default=DefaultEscapeHTML" optval:"param"`
}

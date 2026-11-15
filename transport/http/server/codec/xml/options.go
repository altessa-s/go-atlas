// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package xml

//go:generate go tool optgen generate

// options configures a [Codec].
type options struct {
	// indent ([WithIndent]) enables pretty-printing with 2-space indentation.
	indent bool `optval:"param"`
	// includeHeader ([WithHeader]) controls inclusion of the XML declaration
	// header.
	includeHeader bool `opt:"Header" optval:"param"`
}

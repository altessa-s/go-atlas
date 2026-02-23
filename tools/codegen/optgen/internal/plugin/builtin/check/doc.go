// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package check provides validation check plugins for optgen.
//
// Each plugin emits inline validation code that runs inside the generated
// With* function body, after transforms and before assignment. When the
// option function signature returns an error (--option-error mode) the
// generated code returns a formatted error; otherwise it panics.
//
// Available checks (registered via init):
//
//   - [RequiredCheck] (required / nonempty) -- non-empty/non-nil guard
//   - [MinLenCheck] (minlen=N) -- minimum length for strings, slices, maps
//   - [MaxLenCheck] (maxlen=N) -- maximum length for strings, slices, maps
//   - [OneOfCheck] (oneof=[a,b,...]) -- string allowlist validation
//   - [NonZeroCheck] (nonzero) -- numeric non-zero guard
//
// # Tag Usage
//
//	type Options struct {
//	    Name string `optcheck:"required,minlen=2,maxlen=50"`
//	    Mode string `optcheck:"oneof=[\"fast\",\"slow\"]"`
//	}
package check

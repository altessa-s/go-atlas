// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mapslice provides codecs for converting map values or keys to slices.
//
// Two package-level codecs are available:
//
//   - [Values] extracts map values into a destination slice
//   - [Keys] extracts map keys into a destination slice
//
// Element types must match or the destination must be []any.
// Iteration order follows Go map semantics (non-deterministic).
//
// Example:
//
//	converter.Convert(src, &dst, converter.WithCodecs(mapslice.Values))
package mapslice

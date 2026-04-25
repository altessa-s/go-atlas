// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package postprocess provides modifier plugins that run in [plugin.PhasePostProcess],
// after transforms and before the final struct field assignment.
//
// Built-in post-processors:
//
//   - [DedupModifier] (dedup) -- deduplicates comparable slices; applied by
//     default to all []comparable types, disabled with optval:"nodup"
//   - [NonEmptyModifier] (nonempty) -- for string fields, early-returns on empty;
//     for []string fields, filters out empty elements in-place
//   - [PositiveModifier] (positive) -- early-returns if value is <= 0 (or < 0
//     with positive=allow_zero); applied by default to time.Duration, disabled
//     with optval:"nonpositive"
//
// # Tag Usage
//
//	type Options struct {
//	    Tags    []string      `optval:"dedup,nonempty"`
//	    IDs     []int         `optval:"positive"`
//	    Timeout time.Duration // positive applied automatically
//	}
package postprocess

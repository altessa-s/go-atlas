// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package fieldtracker detects changed fields between two struct instances.
//
// Field paths are returned in dot notation (e.g. "address.city") using names
// derived from struct tags (default "json") or PascalCase-to-snake_case
// conversion. Nested structs, embedded structs, slices, and maps are traversed
// recursively up to a configurable depth (default [DefaultMaxDepth]).
//
// For one-shot comparisons use the package-level [GetChangedFields]. For
// repeated comparisons, create a [Tracker] via [NewTracker] to benefit from
// cached type metadata (10-20% faster).
//
// Example:
//
//	before := &Person{FirstName: "John", Email: "old@example.com"}
//	after := &Person{FirstName: "Jane", Email: "old@example.com"}
//	changed := fieldtracker.GetChangedFields(before, after)
//	// Returns: ["first_name"]
package fieldtracker

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

// Standard timestamp field names in the model AST. These are the
// targets [TimestampFilters] expands its custom functions to. If the
// underlying database column uses a different convention (e.g.
// snake_case created_at), translate at the translator layer via
// [WithFieldMapping] — the AST stays canonical.
const (
	TimestampFieldCreatedAt = "createdAt"
	TimestampFieldUpdatedAt = "updatedAt"
	TimestampFieldDeletedAt = "deletedAt"
)

// CEL function names exposed by [TimestampFilters].
const (
	TimestampFuncCreateAfter  = "createAfter"
	TimestampFuncCreateBefore = "createBefore"
	TimestampFuncUpdateAfter  = "updateAfter"
	TimestampFuncUpdateBefore = "updateBefore"
	TimestampFuncDeleteAfter  = "deleteAfter"
	TimestampFuncDeleteBefore = "deleteBefore"
)

// TimestampFilters returns a ready-made set of [CustomFunction]
// handlers that turn semantic timestamp predicates into ordinary
// comparisons against the standard createdAt/updatedAt/deletedAt
// fields:
//
//	createAfter(t)  → createdAt > t
//	createBefore(t) → createdAt < t
//	updateAfter(t)  → updatedAt > t
//	updateBefore(t) → updatedAt < t
//	deleteAfter(t)  → deletedAt > t
//	deleteBefore(t) → deletedAt < t
//
// The functions are not registered automatically — opt in explicitly,
// either application-wide or per parser:
//
//	// Register once during bootstrap so every parser sees them.
//	if err := filter.RegisterFunctions(filter.TimestampFilters()); err != nil {
//	    panic(err)
//	}
//
//	// Or scope to a single parser.
//	parser, _ := filter.NewParser(
//	    filter.WithCustomFunctions(filter.TimestampFilters()),
//	)
//
// Each call returns a fresh map so callers can mutate it (e.g. delete
// unwanted entries) without affecting subsequent invocations. Field
// names are fixed (camelCase); use [WithFieldMapping] on translators
// when storage columns follow a different convention.
func TimestampFilters() map[string]CustomFunction {
	return map[string]CustomFunction{
		TimestampFuncCreateAfter:  CompareField(TimestampFieldCreatedAt, OpGT),
		TimestampFuncCreateBefore: CompareField(TimestampFieldCreatedAt, OpLT),
		TimestampFuncUpdateAfter:  CompareField(TimestampFieldUpdatedAt, OpGT),
		TimestampFuncUpdateBefore: CompareField(TimestampFieldUpdatedAt, OpLT),
		TimestampFuncDeleteAfter:  CompareField(TimestampFieldDeletedAt, OpGT),
		TimestampFuncDeleteBefore: CompareField(TimestampFieldDeletedAt, OpLT),
	}
}

// SelectTimestampFilters builds a subset of [TimestampFilters]
// containing only the named entries. Names not part of the timestamp
// preset are silently skipped — the helper is for explicit subset
// selection, not validation. Pass the [TimestampFunc*] constants to
// avoid typos.
//
// Useful when a model lacks a soft delete or the API should expose
// only a partial range of predicates:
//
//	filter.RegisterFunctions(filter.SelectTimestampFilters(
//	    filter.TimestampFuncCreateAfter,
//	    filter.TimestampFuncCreateBefore,
//	    filter.TimestampFuncUpdateAfter,
//	    filter.TimestampFuncUpdateBefore,
//	))
//
// Calling with no arguments returns an empty map. Duplicate names are
// deduplicated by the resulting map.
func SelectTimestampFilters(names ...string) map[string]CustomFunction {
	all := TimestampFilters()
	out := make(map[string]CustomFunction, len(names))
	for _, name := range names {
		if h, ok := all[name]; ok {
			out[name] = h
		}
	}
	return out
}

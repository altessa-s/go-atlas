// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/store/mongo/internal/pipeline"
)

// buildCursorFilter resumes after a validated boundary. MongoDB's comparison
// operators only match within the boundary's BSON type, so null and missing
// values need explicit branches when entering or leaving the null segment.
func buildCursorFilter(cursor *Cursor, cursorIdField string, sort bson.D) bson.M {
	if cursor.IsZero() {
		return bson.M{}
	}
	oid, err := bson.ObjectIDFromHex(cursor.CursorId)
	if err != nil {
		return bson.M{}
	}
	descending := getSortDirection(sort) == sortDirectionDescending
	op := "$gt"
	if descending {
		op = "$lt"
	}
	idComparison := bson.M{op: oid}
	if len(sort) == 0 || sort[0].Key == cursorIdField {
		return bson.M{cursorIdField: idComparison}
	}
	value, err := decodeSortValue(cursor.SortValue)
	if err != nil {
		return bson.M{}
	}

	field := sort[0].Key
	if value.Type == bson.TypeNull {
		ties := bson.M{field: nil, cursorIdField: idComparison}
		if descending {
			return ties
		}
		return bson.M{"$or": bson.A{ties, bson.M{field: bson.M{"$ne": nil}}}}
	}

	branches := bson.A{
		bson.M{field: bson.M{op: value}},
		bson.M{opAnd: bson.A{
			// Explicit $eq keeps the value in operand position.
			bson.M{field: bson.M{opEq: value}},
			bson.M{cursorIdField: idComparison},
		}},
	}
	if descending {
		branches = append(branches, bson.M{field: nil})
	}
	return bson.M{"$or": branches}
}

// getSortDirection extracts the sort direction from the primary (first) sort field.
// Cursor-based pagination only uses the primary sort field for cursor generation,
// so this function examines only the first element of the sort specification.
//
// Parameters:
//   - sort: MongoDB sort specification as bson.D
//
// Returns:
//   - 1 if the first field is sorted ascending (value >= 0)
//   - -1 if the first field is sorted descending (value < 0)
//   - -1 as default if sort is empty or invalid
//
// Example:
//   - bson.D{{"created_at", -1}, {"_id", -1}} → -1 (descending)
//   - bson.D{{"name", 1}} → 1 (ascending)
func getSortDirection(sort bson.D) int {
	if len(sort) == 0 {
		return sortDirectionDescending
	}

	// Get direction from the first sort field.
	// Handle multiple integer types because BSON round-trip may change
	// int to int32 (BSON default for small integers).
	switch v := sort[0].Value.(type) {
	case int:
		return dirFromInt(int64(v))
	case int32:
		return dirFromInt(int64(v))
	case int64:
		return dirFromInt(v)
	default:
		return sortDirectionDescending
	}
}

// dirFromInt returns sort direction constant from an integer value.
func dirFromInt(direction int64) int {
	if direction < 0 {
		return sortDirectionDescending
	}
	return sortDirectionAscending
}

// validateCursorSort rejects sorts that buildCursorFilter cannot resume
// correctly. The cursor stores the value of the primary sort field only and
// breaks ties on cursorIdField in the primary direction, so a sort is accepted
// when it is empty (the default applies), a single field, starts with
// cursorIdField (unique, so later keys never apply), or is one field followed
// by cursorIdField in the same direction. Any other secondary key or a
// tiebreaker in the opposite direction would make pages skip or repeat
// documents.
func validateCursorSort(sort bson.D, cursorIdField string) error {
	switch {
	case len(sort) <= 1, sort[0].Key == cursorIdField:
		return nil
	case len(sort) == 2 && sort[1].Key == cursorIdField &&
		getSortDirection(sort[1:]) == getSortDirection(sort):
		return nil
	default:
		return fmt.Errorf("%w: %v (cursor ID field %q)", ErrUnsupportedCursorSort, sort, cursorIdField)
	}
}

// ensureCursorIdInSort returns a sort specification guaranteed to include cursorIdField
// as the final tiebreaker. When multiple documents share the same primary sort value
// (e.g. bulk-inserted records with identical created_at), MongoDB returns them in
// undefined order — causing cursor-based pagination to skip or repeat documents.
//
// The tiebreaker direction matches the primary sort field.
// If cursorIdField is already present, the original slice is returned unchanged.
func ensureCursorIdInSort(sort bson.D, cursorIdField string) bson.D {
	for _, field := range sort {
		if field.Key == cursorIdField {
			return sort // already present
		}
	}

	// Append cursor_id with same direction as primary sort field
	direction := getSortDirection(sort)
	result := make(bson.D, len(sort), len(sort)+1)
	copy(result, sort)
	return append(result, bson.E{Key: cursorIdField, Value: int32(direction)})
}

// buildMatchStage creates the $match stage combining user filter and cursor filter.
// The match stage is critical for index usage and should come before $sort.
//
// Parameters:
//   - filter: User-provided filter conditions
//   - cursor: Current cursor for pagination (nil for first page)
//   - cursorIdField: Field used for cursor ID (e.g., "cursor_id")
//   - sort: Sort specification to determine direction
//
// Returns:
//   - bson.A: Pipeline stages (empty if no filters)
func buildMatchStage(filter bson.Raw, cursor *Cursor, cursorIdField string, sort bson.D) bson.A {
	// Add cursor filter if cursor is provided
	if cursor != nil && !cursor.IsZero() {
		cursorFilter := buildCursorFilter(cursor, cursorIdField, sort)

		// Merge cursor filter with existing filter using $and
		if len(filter) > pipeline.EmptyDocumentSize {
			// Wrap both filters in $and
			return bson.A{
				bson.M{
					opMatch: bson.M{
						opAnd: bson.A{filter, cursorFilter},
					},
				},
			}
		}
		return bson.A{bson.M{opMatch: cursorFilter}}
	}

	if len(filter) > pipeline.EmptyDocumentSize {
		return bson.A{bson.M{opMatch: filter}}
	}

	return bson.A{}
}

// buildSortStage creates the $sort stage.
// Uses the provided sort specification or falls back to default sort.
//
// Parameters:
//   - sort: Sort specification (bson.D)
//
// Returns:
//   - bson.A: Pipeline stages with $sort
func buildSortStage(sort bson.D) bson.A {
	sortStage := sort
	if len(sortStage) == 0 {
		sortStage = DefaultListSort
	}
	return bson.A{bson.M{"$sort": sortStage}}
}

// buildCursorPipeline constructs an optimized MongoDB aggregation pipeline for cursor-based pagination.
// The pipeline is carefully ordered for optimal performance with proper index utilization.
//
// Pipeline stages (in order):
//  1. $match: Combines user filter with cursor filter (if cursor provided)
//  2. $sort: Orders results by the specified sort fields
//  3. Custom stages (via WithListCursorStages): operate on entire result set
//  4. Items aggregation: $limit (fetch limit+1), decoration stages (via WithListCursorDecorationStages), optional $project
//  5. Count + final shape: $replaceRoot + $unionWith + $group + $project, see pipeline.CursorResult
//
// The pipeline fetches limit+1 items to efficiently determine if there are more pages
// without requiring a separate count query. The count branch uses $unionWith with an
// independent sub-pipeline scoped to userFilter only, so total stays stable across pages.
//
// Parameters:
//   - opts: Configuration options including filter, sort, limit, cursor, projection, etc.
//   - collectionName: Name of the collection being queried; required by $unionWith.coll
//
// Returns:
//   - bson.A: Complete aggregation pipeline ready for execution
//
// Performance characteristics:
//   - O(1) pagination depth (unlike offset-based which is O(n))
//   - Single database round-trip: items + count travel together as one aggregation
//   - The $unionWith count branch re-applies userFilter against the same collection, so
//     ensure the user filter is index-backed — otherwise the count branch costs a full
//     collection scan in addition to the items branch
//   - Efficient index usage with proper $match before $sort
func buildCursorPipeline(opts *listCursorOptions, collectionName string) bson.A {
	// Extend sort with cursor_id tiebreaker for deterministic pagination.
	// A local variable is used intentionally — opts.sort must stay unchanged
	// because cursor generation and checksum validation depend on the original value.
	sort := ensureCursorIdInSort(opts.sort, opts.cursorIdField)

	// 1. MATCH - Combine user filter with cursor filter
	match := buildMatchStage(
		opts.prepared,
		opts.cursor,
		opts.cursorIdField,
		sort, // local sort with tiebreaker
	)

	// 2. SORT - Order results
	sortStage := buildSortStage(sort)

	// 3. CUSTOM STAGES - User-provided stages after $sort, before items aggregation
	stages := opts.stages

	// 4. ITEMS - $limit (fetch limit+1) + decoration stages + optional projection
	items := pipeline.CursorItems(
		opts.limit,
		opts.projection,
		opts.decorationStages,
	)

	// 5. COUNT + FINAL SHAPE - tag items, join independent count, collapse into {items, total}
	count := pipeline.CursorResult(
		collectionName,
		opts.prepared,
		opts.includeTotal,
	)

	pipeline := make(bson.A, 0, len(match)+len(sortStage)+len(stages)+len(items)+len(count))
	pipeline = append(pipeline, match...)
	pipeline = append(pipeline, sortStage...)
	pipeline = append(pipeline, stages...)
	pipeline = append(pipeline, items...)
	pipeline = append(pipeline, count...)

	return pipeline
}

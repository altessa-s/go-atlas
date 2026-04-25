// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"maps"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/core/collections/slices"
)

// buildCursorFilter constructs a MongoDB filter for cursor-based pagination using cursor_id.
// Since cursor_id (ObjectID) is lexicographically sortable and contains timestamp, we only need
// to compare cursor_id values without separate timestamp comparison.
//
// When SortValue is present in the cursor, uses compound filter for correct pagination.
// This handles cases where sort field order differs from cursor_id order.
//
// Parameters:
//   - cursor: The pagination cursor containing cursor_id from the last item
//   - cursorIdField: The MongoDB field name containing the cursor ID (e.g., "cursor_id", "_id")
//   - sort: MongoDB sort specification as bson.D
//
// Returns:
//   - Empty bson.M if cursor is nil or zero
//   - Filter using $lt/$gt on cursor_id field based on sort direction
//   - Compound filter if SortValue is present
//
// Example output for descending sort:
//
//	{"cursor_id": {"$lt": "01HQVZX3M8..."}}
//
// Example output for ascending sort:
//
//	{"cursor_id": {"$gt": "01HQVZX3M8..."}}
//
// Example output with SortValue (compound filter):
//
//	{"$or": [
//	    {"updated_at": {"$lt": 1234567890}},
//	    {"$and": [
//	        {"updated_at": 1234567890},
//	        {"cursor_id": {"$lt": "01HQVZX3M8..."}}
//	    ]}
//	]}
func buildCursorFilter(cursor *Cursor, cursorIdField string, sort bson.D) bson.M {
	if cursor == nil || cursor.IsZero() {
		return bson.M{}
	}

	sortDirection := getSortDirection(sort)

	// Determine comparison operator based on sort direction
	// For descending sort: we want items BEFORE the cursor (cursor_id < cursor)
	// For ascending sort: we want items AFTER the cursor (cursor_id > cursor)
	var op string
	if sortDirection == sortDirectionDescending {
		op = "$lt" // $lt for descending
	} else {
		op = "$gt" // $gt for ascending
	}

	// At this point, cursor.CursorId is already validated by ValidateChecksum,
	// so ObjectIDFromHex should never fail. If it does, it indicates a serious bug.
	oid, err := bson.ObjectIDFromHex(cursor.CursorId)
	if err != nil {
		// Defensive: return empty filter to fail gracefully instead of crashing.
		// This should never happen since cursor was validated, but library code
		// must not panic. Returning empty filter will cause pagination to restart.
		return bson.M{}
	}

	// When SortValue is present, use compound filter for correct pagination.
	// This handles cases where sort field order differs from cursor_id order.
	// Filter: (sort_field < value) OR (sort_field == value AND cursor_id < id)
	if cursor.SortValue != "" && len(sort) > 0 {
		sortFieldName := sort[0].Key

		// Decode sort value from base64 BSON to preserve type precision
		sortValue, err := decodeSortValue(cursor.SortValue)
		if err != nil {
			// Defensive: return empty filter to fail gracefully
			// This should never happen if cursor was properly created
			return bson.M{}
		}

		return bson.M{
			"$or": bson.A{
				// Items with smaller/larger sort value (depending on direction)
				bson.M{sortFieldName: bson.M{op: sortValue}},
				// Items with same sort value but smaller/larger cursor_id (tie-breaker)
				bson.M{
					"$and": bson.A{
						bson.M{sortFieldName: sortValue},
						bson.M{cursorIdField: bson.M{op: oid}},
					},
				},
			},
		}
	}

	return bson.M{
		cursorIdField: bson.M{op: oid},
	}
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
func buildMatchStage(filter bson.M, cursor *Cursor, cursorIdField string, sort bson.D) bson.A {
	combinedFilter := bson.M{}

	// Add user-provided filter
	maps.Copy(combinedFilter, filter)

	// Add cursor filter if cursor is provided
	if cursor != nil && !cursor.IsZero() {
		cursorFilter := buildCursorFilter(cursor, cursorIdField, sort)

		// Merge cursor filter with existing filter using $and
		if len(combinedFilter) > 0 {
			// Wrap both filters in $and
			return bson.A{
				bson.M{
					"$match": bson.M{
						"$and": bson.A{combinedFilter, cursorFilter},
					},
				},
			}
		}
		return bson.A{bson.M{"$match": cursorFilter}}
	}

	if len(combinedFilter) > 0 {
		return bson.A{bson.M{"$match": combinedFilter}}
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

// totalAnnotationField is the temporary field name used by $setWindowFields to store
// the pre-computed document count before custom pipeline stages are applied.
// This ensures accurate totals even when custom stages change document cardinality (e.g., $unwind).
const totalAnnotationField = "__atlas_total"

// buildFacetStage creates the $facet stage for parallel items and count queries.
// Fetches limit+1 items to determine if there are more pages.
//
// Parameters:
//   - limit: Maximum number of items to return
//   - projection: Optional field projection
//   - includeTotal: Whether to include total count
//   - decorationStages: Optional pipeline stages inserted after $limit (e.g., $lookup for display data)
//   - preCountedTotal: When true, total was pre-computed via $setWindowFields and stored in
//     totalAnnotationField; the count branch reads it instead of running $count
//
// Returns:
//   - bson.A: Pipeline stages with $facet
func buildFacetStage(limit int64, projection bson.M, includeTotal bool, decorationStages bson.A, preCountedTotal bool) bson.A {
	// Build items pipeline: limit + lookahead + decoration stages + optional projection
	itemsPipeline := bson.A{
		bson.M{"$limit": limit + paginationLookaheadCount},
	}

	itemsPipeline = append(itemsPipeline, decorationStages...)

	// Remove the annotation field so it doesn't leak into results
	if preCountedTotal {
		itemsPipeline = append(itemsPipeline, bson.M{"$unset": totalAnnotationField})
	}

	itemsPipeline = slices.AppendIf[any](itemsPipeline, projection != nil, bson.M{"$project": projection})

	// Build facet stage
	facetStage := bson.M{
		"items": itemsPipeline,
	}

	if includeTotal {
		if preCountedTotal {
			// Read the pre-computed count from the annotation field.
			// Any single document carries the correct value, so $limit: 1 is sufficient.
			facetStage["count"] = bson.A{
				bson.M{"$limit": int64(1)},
				bson.M{"$project": bson.M{"total": "$" + totalAnnotationField}},
			}
		} else {
			facetStage["count"] = bson.A{bson.M{"$count": "total"}}
		}
	}

	return bson.A{bson.M{"$facet": facetStage}}
}

// buildCursorPipeline constructs an optimized MongoDB aggregation pipeline for cursor-based pagination.
// The pipeline is carefully ordered for optimal performance with proper index utilization.
//
// Pipeline stages (in order):
//  1. $match: Combines user filter with cursor filter (if cursor provided)
//  2. $sort: Orders results by the specified sort fields
//  3. $setWindowFields: Pre-computes total count (only when custom stages + includeTotal, requires MongoDB 5.0+)
//  4. Custom stages (via WithListCursorStages): operate on entire result set
//  5. $facet: Splits into parallel pipelines for items and count
//     - items: $limit (fetch limit+1) + decoration stages (via WithListCursorDecorationStages) + $project (optional)
//     - count: $count or pre-computed total (total documents matching filter)
//  6. $unwind + $project: Transforms facet output into {items: [], total: N} structure
//
// The pipeline fetches limit+1 items to efficiently determine if there are more pages
// without requiring a separate count query. If we get more than limit items, we know
// there's a next page and can construct the cursor from the last visible item.
//
// Parameters:
//   - opts: Configuration options including filter, sort, limit, cursor, projection, etc.
//
// Returns:
//   - bson.A: Complete aggregation pipeline ready for execution
//
// Performance characteristics:
//   - O(1) pagination depth (unlike offset-based which is O(n))
//   - Single database round-trip for items + count
//   - Efficient index usage with proper $match before $sort
func buildCursorPipeline(opts *listCursorOptions) bson.A {
	// Extend sort with cursor_id tiebreaker for deterministic pagination.
	// A local variable is used intentionally — opts.sort must stay unchanged
	// because cursor generation and checksum validation depend on the original value.
	sort := ensureCursorIdInSort(opts.sort, opts.cursorIdField)

	pipeline := bson.A{}

	// 1. MATCH - Combine user filter with cursor filter
	pipeline = append(pipeline, buildMatchStage(
		opts.filter,
		opts.cursor,
		opts.cursorIdField,
		sort, // local sort with tiebreaker
	)...)

	// 2. SORT - Order results
	pipeline = append(pipeline, buildSortStage(sort)...)

	// 3. PRE-COUNT TOTAL - When custom stages may change document cardinality (e.g., $unwind),
	// annotate each document with the true count BEFORE stages are applied.
	// Uses $setWindowFields (requires MongoDB 5.0+) to avoid materializing documents.
	preCountedTotal := len(opts.stages) > 0 && opts.includeTotal
	if preCountedTotal {
		pipeline = append(pipeline, bson.M{
			"$setWindowFields": bson.M{
				"output": bson.M{
					totalAnnotationField: bson.M{"$count": bson.M{}},
				},
			},
		})
	}

	// 4. CUSTOM STAGES - User-provided stages after $sort, before $facet
	pipeline = append(pipeline, opts.stages...)

	// 5. FACET - Split into items and count branches
	pipeline = append(pipeline, buildFacetStage(
		opts.limit,
		opts.projection,
		opts.includeTotal,
		opts.decorationStages,
		preCountedTotal,
	)...)

	// 6. UNWIND AND PROJECT - Transform facet results
	return appendFacetResultTransform(pipeline, opts.includeTotal)
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pipeline

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas-core/collections/slices"
)

// Internal field names used by the $unionWith-based count pipeline.
// kindField and itemEnvelopeField are double-underscored to avoid
// collision with user document fields — both surface inside $replaceRoot
// expressions that wrap arbitrary $$ROOT documents, so any clash would
// silently lose user data. The envelope is unwrapped via $cond + $$REMOVE
// before results leave the pipeline, but the safety guarantee has to
// hold INSIDE the pipeline regardless.
//
// kindItem / kindCount are tag values, not field names — they don't need
// the underscore guard because they're only ever compared with $eq, never
// used as document keys.
const (
	// kindField tags each document in the union as either an item or the count result.
	kindField = "__kind"
	kindItem  = "item"
	kindCount = "count"
	// itemEnvelopeField wraps the original item document so the tag does not pollute its fields.
	itemEnvelopeField = "__item"
)

// projectStageOperator selects the output document fields.
const projectStageOperator = "$project"

// countStageOperator counts documents in a pipeline.
const countStageOperator = "$count"

// itemsField holds the paginated result documents.
const itemsField = "items"

// totalField holds the result count or the sentinel for an unrequested count.
const totalField = "total"

// idField is MongoDB’s document or group identifier.
const idField = "_id"

// itemsReference references the paginated result array.
const itemsReference = "$items"

// countReference references the count facet array.
const countReference = "$count"

// Offset builds an offset pagination pipeline with an optional total count.
// The caller supplies the effective sort and validated pagination bounds.
func Offset(filter bson.M, sort bson.D, projection bson.M, limit, offset int64, includeTotal bool) bson.A {
	pipeline := bson.A{}

	// 1. MATCH FIRST - Filter early to reduce the dataset size
	pipeline = slices.AppendIf[any](pipeline, len(filter) > 0, bson.M{"$match": filter})

	// 2. SORT AFTER FILTERING - Better index utilization when combined with match
	pipeline = append(pipeline, bson.M{"$sort": sort})

	// 3. FACET - Split into parallel pipelines for items and count
	// This allows us to get both paginated results and total count efficiently
	itemsPipeline := bson.A{}

	// Add skip/limit for pagination in the items branch
	itemsPipeline = slices.AppendIf[any](itemsPipeline, offset > 0, bson.M{"$skip": offset})
	itemsPipeline = append(itemsPipeline, bson.M{"$limit": limit})

	// Add projection if specified (after pagination to reduce data transfer)
	itemsPipeline = slices.AppendIf[any](itemsPipeline, len(projection) > 0, bson.M{projectStageOperator: projection})

	// Facet stage with optimized branches
	facetStage := bson.M{
		itemsField: itemsPipeline,
	}
	if includeTotal {
		facetStage["count"] = bson.A{bson.M{countStageOperator: totalField}}
	}

	pipeline = append(pipeline, bson.M{"$facet": facetStage})

	// 4. UNWIND AND PROJECT - Transform facet results into final structure
	return appendFacetResultTransform(pipeline, includeTotal)
}

// CursorItems builds the items portion of the pipeline:
// $limit (with lookahead), decoration stages, and an optional $project.
// Fetches limit+1 items to determine if there are more pages.
//
// Parameters:
//   - limit: Maximum number of items to return
//   - projection: Optional field projection
//   - decorationStages: Optional pipeline stages inserted after $limit (e.g., $lookup for display data)
//
// Returns:
//   - bson.A: Pipeline stages for the items branch
func CursorItems(limit int64, projection bson.M, decorationStages bson.A) bson.A {
	stages := bson.A{
		bson.M{"$limit": limit + 1},
	}
	stages = append(stages, decorationStages...)
	stages = slices.AppendIf[any](stages, len(projection) > 0, bson.M{projectStageOperator: projection})
	return stages
}

// CursorResult appends the stages that produce the final {items, total} envelope.
// The count is computed via $unionWith with an independent sub-pipeline scoped to userFilter
// only (no cursor filter, no custom stages), so total stays stable across pages.
// When includeTotal is false, items are collapsed and total is set to -1.
//
// Parameters:
//   - collectionName: Name of the collection being queried; used in $unionWith.coll
//   - userFilter: User filter applied to the count sub-pipeline (cursor filter must not leak here)
//   - includeTotal: Whether to compute the total count
//
// Returns:
//   - bson.A: Stages that shape the result into {items, total}
func CursorResult(collectionName string, userFilter bson.M, includeTotal bool) bson.A {
	if !includeTotal {
		// No counting: collapse items into {items, total: -1}.
		return bson.A{
			bson.M{"$group": bson.M{
				idField:    nil,
				itemsField: bson.M{"$push": "$$ROOT"},
			}},
			bson.M{projectStageOperator: bson.M{
				idField:    0,
				itemsField: itemsReference,
				totalField: int64(-1),
			}},
		}
	}

	// Tag each item so $group below can distinguish it from the count entry.
	tagItem := bson.M{"$replaceRoot": bson.M{"newRoot": bson.M{
		itemEnvelopeField: "$$ROOT",
		kindField:         kindItem,
	}}}

	// Count sub-pipeline: independent query scoped to userFilter only.
	countSubPipeline := bson.A{}
	countSubPipeline = slices.AppendIf[any](countSubPipeline, len(userFilter) > 0, bson.M{"$match": userFilter})
	countSubPipeline = append(countSubPipeline,
		bson.M{countStageOperator: totalField},
		bson.M{"$replaceRoot": bson.M{"newRoot": bson.M{
			totalField: "$total",
			kindField:  kindCount,
		}}},
	)

	unionStage := bson.M{"$unionWith": bson.M{
		"coll":     collectionName,
		"pipeline": countSubPipeline,
	}}

	// $$REMOVE skips the push so the items array never contains the count entry.
	groupStage := bson.M{"$group": bson.M{
		idField: nil,
		itemsField: bson.M{"$push": bson.M{"$cond": bson.A{
			bson.M{"$eq": bson.A{"$" + kindField, kindItem}},
			"$" + itemEnvelopeField,
			"$$REMOVE",
		}}},
		"totalValue": bson.M{"$max": "$total"},
	}}

	// $ifNull defaults total to 0 when the collection is empty (no count documents).
	projectStage := bson.M{projectStageOperator: bson.M{
		idField:    0,
		itemsField: itemsReference,
		totalField: bson.M{"$ifNull": bson.A{"$totalValue", int64(0)}},
	}}

	return bson.A{tagItem, unionStage, groupStage, projectStage}
}

// appendFacetResultTransform normalizes facet aggregation results into a consistent structure.
// MongoDB's $facet stage returns results in a nested structure like:
//
//	{items: [{...}, {...}], count: [{total: 100}]}
//
// This function appends pipeline stages that transform it into a flat, predictable format:
//
//	{items: [{...}, {...}], total: 100}
//
// Parameters:
//   - pipeline: The existing aggregation pipeline (must end with a $facet stage)
//   - includeTotal: Whether the facet includes a count branch
//
// Returns:
//   - bson.A: Pipeline with additional $unwind and $project stages appended
//
// Transformation logic:
//   - If includeTotal is true:
//     1. $unwind the count array (preserving null/empty arrays)
//     2. $project to extract count.total with $ifNull default to 0
//   - If includeTotal is false:
//     1. $project to set total to -1 (indicates count was not computed)
//
// The resulting structure is:
//   - items: Array of documents (always present, may be empty)
//   - total: Integer count (>=0 if includeTotal true, -1 if false)
//
// Use case: Standardizing pagination results across list operations for consistent
// API responses and easier client-side handling.
func appendFacetResultTransform(pipeline bson.A, includeTotal bool) bson.A {
	if includeTotal {
		return append(pipeline,
			bson.M{"$unwind": bson.M{
				"path":                       countReference,
				"preserveNullAndEmptyArrays": true,
			}},
			bson.M{projectStageOperator: bson.M{
				totalField: bson.M{"$ifNull": bson.A{"$count.total", 0}},
				itemsField: itemsReference,
			}},
		)
	}

	return append(pipeline,
		bson.M{projectStageOperator: bson.M{
			totalField: -1,
			itemsField: itemsReference,
		}},
	)
}

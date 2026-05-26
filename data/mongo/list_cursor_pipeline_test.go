// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestEnsureCursorIdInSort(t *testing.T) {
	tests := []struct {
		name          string
		sort          bson.D
		cursorIdField string
		wantLen       int
		wantLastKey   string
		wantLastVal   int32
		wantSameSlice bool // expect original slice returned unchanged
	}{
		{
			name:          "appends cursor_id with descending direction",
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
			cursorIdField: "cursor_id",
			wantLen:       2,
			wantLastKey:   "cursor_id",
			wantLastVal:   int32(sortDirectionDescending),
		},
		{
			name:          "appends cursor_id with ascending direction",
			sort:          bson.D{{Key: "name", Value: int32(1)}},
			cursorIdField: "cursor_id",
			wantLen:       2,
			wantLastKey:   "cursor_id",
			wantLastVal:   int32(sortDirectionAscending),
		},
		{
			name:          "already present - returns original",
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}, {Key: "cursor_id", Value: int32(-1)}},
			cursorIdField: "cursor_id",
			wantLen:       2,
			wantSameSlice: true,
		},
		{
			name:          "cursor_id as only sort field - returns original",
			sort:          bson.D{{Key: "cursor_id", Value: int32(1)}},
			cursorIdField: "cursor_id",
			wantLen:       1,
			wantSameSlice: true,
		},
		{
			name:          "custom cursor id field",
			sort:          bson.D{{Key: "updated_at", Value: int32(-1)}},
			cursorIdField: "_id",
			wantLen:       2,
			wantLastKey:   "_id",
			wantLastVal:   int32(sortDirectionDescending),
		},
		{
			name:          "multiple sort fields without cursor_id",
			sort:          bson.D{{Key: "category", Value: int32(1)}, {Key: "name", Value: int32(-1)}},
			cursorIdField: "cursor_id",
			wantLen:       3,
			wantLastKey:   "cursor_id",
			wantLastVal:   int32(sortDirectionAscending), // matches primary field direction
		},
		{
			name:          "empty sort appends with default descending direction",
			sort:          bson.D{},
			cursorIdField: "cursor_id",
			wantLen:       1,
			wantLastKey:   "cursor_id",
			wantLastVal:   int32(sortDirectionDescending),
		},
		{
			name:          "int value sort field",
			sort:          bson.D{{Key: "created_at", Value: -1}},
			cursorIdField: "cursor_id",
			wantLen:       2,
			wantLastKey:   "cursor_id",
			wantLastVal:   int32(sortDirectionDescending),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ensureCursorIdInSort(tt.sort, tt.cursorIdField)

			require.Len(t, got, tt.wantLen, "got %v", got)

			if tt.wantSameSlice {
				require.True(t, &got[0] == &tt.sort[0], "expected same underlying slice, got a copy")
				return
			}

			last := got[len(got)-1]
			require.Equal(t, tt.wantLastKey, last.Key, "last key")
			require.Equal(t, tt.wantLastVal, last.Value, "last value")
		})
	}
}

func TestEnsureCursorIdInSort_DoesNotMutateOriginal(t *testing.T) {
	original := bson.D{{Key: "created_at", Value: int32(-1)}}
	originalCopy := make(bson.D, len(original))
	copy(originalCopy, original)

	result := ensureCursorIdInSort(original, "cursor_id")

	// Original must be unchanged
	require.Len(t, original, len(originalCopy), "original length changed")
	for i := range original {
		require.Equal(t, originalCopy[i].Key, original[i].Key, "original[%d] key mutated", i)
		require.Equal(t, originalCopy[i].Value, original[i].Value, "original[%d] value mutated", i)
	}

	// Result must be a different slice
	require.Len(t, result, 2)
}

func TestBuildSortStage(t *testing.T) {
	tests := []struct {
		name     string
		sort     bson.D
		wantSort bson.D
	}{
		{
			name:     "uses provided sort",
			sort:     bson.D{{Key: "name", Value: 1}},
			wantSort: bson.D{{Key: "name", Value: 1}},
		},
		{
			name:     "empty sort falls back to default",
			sort:     bson.D{},
			wantSort: DefaultListSort,
		},
		{
			name:     "nil sort falls back to default",
			sort:     nil,
			wantSort: DefaultListSort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSortStage(tt.sort)
			require.Len(t, got, 1)
			stage, ok := got[0].(bson.M)
			require.True(t, ok, "stage is not bson.M")
			sortSpec, ok := stage["$sort"].(bson.D)
			require.True(t, ok, "$sort value is not bson.D")
			assertBsonDEqual(t, tt.wantSort, sortSpec)
		})
	}
}

func TestBuildMatchStage(t *testing.T) {
	t.Run("no filter no cursor", func(t *testing.T) {
		got := buildMatchStage(bson.M{}, nil, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		require.Empty(t, got)
	})

	t.Run("filter only", func(t *testing.T) {
		filter := bson.M{"status": "active"}
		got := buildMatchStage(filter, nil, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		require.Len(t, got, 1)
		stage := got[0].(bson.M)
		match := stage["$match"].(bson.M)
		require.Equal(t, "active", match["status"], "filter not applied")
	})

	t.Run("cursor only", func(t *testing.T) {
		oid := bson.NewObjectID()
		cursor := &Cursor{CursorId: oid.Hex()}
		got := buildMatchStage(bson.M{}, cursor, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		require.Len(t, got, 1)
		stage := got[0].(bson.M)
		match := stage["$match"].(bson.M)
		require.Contains(t, match, "cursor_id", "cursor filter not applied")
	})

	t.Run("filter and cursor combined with $and", func(t *testing.T) {
		oid := bson.NewObjectID()
		cursor := &Cursor{CursorId: oid.Hex()}
		filter := bson.M{"status": "active"}
		got := buildMatchStage(filter, cursor, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		require.Len(t, got, 1)
		stage := got[0].(bson.M)
		match := stage["$match"].(bson.M)
		require.Contains(t, match, "$and", "expected $and operator for combined filter")
	})

	t.Run("zero cursor treated as no cursor", func(t *testing.T) {
		cursor := &Cursor{} // zero cursor
		got := buildMatchStage(bson.M{}, cursor, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		require.Empty(t, got, "expected empty pipeline for zero cursor")
	})
}

func TestBuildCursorFilter(t *testing.T) {
	oid := bson.NewObjectID()

	t.Run("nil cursor returns empty", func(t *testing.T) {
		got := buildCursorFilter(nil, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		require.Empty(t, got)
	})

	t.Run("zero cursor returns empty", func(t *testing.T) {
		got := buildCursorFilter(&Cursor{}, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		require.Empty(t, got)
	})

	t.Run("descending sort uses $lt", func(t *testing.T) {
		cursor := &Cursor{CursorId: oid.Hex()}
		got := buildCursorFilter(cursor, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		cursorFilter, ok := got["cursor_id"].(bson.M)
		require.True(t, ok, "expected cursor_id filter, got %v", got)
		require.Contains(t, cursorFilter, "$lt", "expected $lt for descending sort")
	})

	t.Run("ascending sort uses $gt", func(t *testing.T) {
		cursor := &Cursor{CursorId: oid.Hex()}
		got := buildCursorFilter(cursor, "cursor_id", bson.D{{Key: "created_at", Value: 1}})
		cursorFilter, ok := got["cursor_id"].(bson.M)
		require.True(t, ok, "expected cursor_id filter, got %v", got)
		require.Contains(t, cursorFilter, "$gt", "expected $gt for ascending sort")
	})

	t.Run("with sort value uses compound $or filter", func(t *testing.T) {
		sortVal, err := encodeSortValue(int64(1234567890))
		require.NoError(t, err, "encodeSortValue")
		cursor := &Cursor{CursorId: oid.Hex(), SortValue: sortVal}
		got := buildCursorFilter(cursor, "cursor_id", bson.D{{Key: "updated_at", Value: -1}})
		require.Contains(t, got, "$or", "expected $or filter for compound sort")
	})

	t.Run("invalid cursor id returns empty", func(t *testing.T) {
		cursor := &Cursor{CursorId: "invalid-hex"}
		got := buildCursorFilter(cursor, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		require.Empty(t, got, "expected empty filter for invalid cursor id")
	})
}

// testCollectionName is the collection name used in pipeline tests; surfaces in $unionWith.coll.
const testCollectionName = "test_collection"

func TestBuildItemsAggregationStages(t *testing.T) {
	t.Run("limit only", func(t *testing.T) {
		got := buildItemsAggregationStages(10, nil, nil)
		require.Len(t, got, 1)
		assertBsonDStageKey(t, got[0], "$limit")
		require.Equal(t, int64(11), got[0].(bson.M)["$limit"], "limit should be 10 + lookahead")
	})

	t.Run("limit + decoration", func(t *testing.T) {
		decoration := bson.A{
			bson.D{{Key: "$lookup", Value: bson.M{"from": "translations", "localField": "_id", "foreignField": "entity_id", "as": "translations"}}},
		}
		got := buildItemsAggregationStages(10, nil, decoration)
		require.Len(t, got, 2)
		assertBsonDStageKey(t, got[0], "$limit")
		assertBsonDStageKey(t, got[1], "$lookup")
	})

	t.Run("limit + decoration + projection", func(t *testing.T) {
		decoration := bson.A{bson.D{{Key: "$lookup", Value: bson.M{"from": "translations"}}}}
		proj := bson.M{"name": 1}
		got := buildItemsAggregationStages(10, proj, decoration)
		require.Len(t, got, 3)
		assertBsonDStageKey(t, got[0], "$limit")
		assertBsonDStageKey(t, got[1], "$lookup")
		assertBsonDStageKey(t, got[2], "$project")
	})

	t.Run("limit + projection (no decoration)", func(t *testing.T) {
		got := buildItemsAggregationStages(10, bson.M{"name": 1}, nil)
		require.Len(t, got, 2)
		assertBsonDStageKey(t, got[0], "$limit")
		assertBsonDStageKey(t, got[1], "$project")
	})
}

func TestBuildCountUnionStages(t *testing.T) {
	t.Run("includeTotal=false: only $group + $project total=-1", func(t *testing.T) {
		got := buildCountUnionStages(testCollectionName, bson.M{"status": "active"}, false)
		require.Len(t, got, 2)
		assertBsonDStageKey(t, got[0], "$group")
		assertBsonDStageKey(t, got[1], "$project")

		// Final $project must set total to -1 (sentinel for "not computed").
		proj := got[1].(bson.M)["$project"].(bson.M)
		require.EqualValues(t, -1, proj["total"], "total should be -1 when includeTotal is false")
	})

	t.Run("includeTotal=true: $replaceRoot + $unionWith + $group + $project", func(t *testing.T) {
		got := buildCountUnionStages(testCollectionName, bson.M{"status": "active"}, true)
		require.Len(t, got, 4)
		assertBsonDStageKey(t, got[0], "$replaceRoot")
		assertBsonDStageKey(t, got[1], "$unionWith")
		assertBsonDStageKey(t, got[2], "$group")
		assertBsonDStageKey(t, got[3], "$project")
	})

	t.Run("$unionWith targets the same collection", func(t *testing.T) {
		got := buildCountUnionStages(testCollectionName, bson.M{"status": "active"}, true)
		union := got[1].(bson.M)["$unionWith"].(bson.M)
		require.Equal(t, testCollectionName, union["coll"], "$unionWith should target the originating collection")
	})

	t.Run("count sub-pipeline contains user filter but never cursor filter", func(t *testing.T) {
		userFilter := bson.M{"status": "active"}
		got := buildCountUnionStages(testCollectionName, userFilter, true)
		subPipeline := got[1].(bson.M)["$unionWith"].(bson.M)["pipeline"].(bson.A)

		// Expected: $match (userFilter), $count, $replaceRoot (tag as "count").
		require.Len(t, subPipeline, 3)
		matchStage := subPipeline[0].(bson.M)["$match"].(bson.M)
		require.Equal(t, "active", matchStage["status"], "$match should carry the user filter")
		require.NotContains(t, matchStage, "$and", "user filter should not be wrapped in $and (no cursor_filter expected)")
		require.NotContains(t, matchStage, "cursor_id", "cursor filter must not leak into count sub-pipeline")
		assertBsonDStageKey(t, subPipeline[1], "$count")
		assertBsonDStageKey(t, subPipeline[2], "$replaceRoot")
	})

	t.Run("empty user filter omits $match in count sub-pipeline", func(t *testing.T) {
		got := buildCountUnionStages(testCollectionName, bson.M{}, true)
		subPipeline := got[1].(bson.M)["$unionWith"].(bson.M)["pipeline"].(bson.A)

		// Expected: $count, $replaceRoot — no $match when filter is empty.
		require.Len(t, subPipeline, 2)
		assertBsonDStageKey(t, subPipeline[0], "$count")
		assertBsonDStageKey(t, subPipeline[1], "$replaceRoot")
	})
}

func TestBuildCursorPipeline(t *testing.T) {
	t.Run("basic pipeline starts with $match then $sort, sort has cursor_id tiebreaker", func(t *testing.T) {
		opts := &listCursorOptions{
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
			filter:        bson.M{"status": "active"},
			limit:         10,
			cursorIdField: "cursor_id",
			includeTotal:  true,
		}

		pipeline := buildCursorPipeline(opts, testCollectionName)

		require.GreaterOrEqual(t, len(pipeline), 3)
		require.Contains(t, pipeline[0].(bson.M), "$match", "first stage should be $match")

		sortStage, ok := pipeline[1].(bson.M)["$sort"]
		require.True(t, ok, "second stage should be $sort")
		sortDoc := sortStage.(bson.D)
		lastField := sortDoc[len(sortDoc)-1]
		require.Equal(t, "cursor_id", lastField.Key, "last sort field should be cursor_id")
	})

	t.Run("opts.sort is not mutated by tiebreaker addition", func(t *testing.T) {
		opts := &listCursorOptions{
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
			filter:        bson.M{},
			limit:         10,
			cursorIdField: "cursor_id",
		}
		originalSortLen := len(opts.sort)

		buildCursorPipeline(opts, testCollectionName)

		require.Len(t, opts.sort, originalSortLen, "opts.sort was mutated")
	})

	t.Run("sort already containing cursor_id is not modified", func(t *testing.T) {
		opts := &listCursorOptions{
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}, {Key: "cursor_id", Value: int32(-1)}},
			filter:        bson.M{},
			limit:         10,
			cursorIdField: "cursor_id",
		}

		pipeline := buildCursorPipeline(opts, testCollectionName)

		sortStage := pipeline[0].(bson.M)["$sort"].(bson.D) // $sort is first when no filter/cursor
		require.Len(t, sortStage, 2, "sort should still have 2 fields")
	})

	t.Run("includeTotal=false omits $unionWith and $replaceRoot tag", func(t *testing.T) {
		opts := baseCursorOpts()

		pipeline := buildCursorPipeline(opts, testCollectionName)

		require.Less(t, findStageIndex(pipeline, "$unionWith"), 0, "$unionWith must not be present when includeTotal is false")
		require.Less(t, findStageIndex(pipeline, "$replaceRoot"), 0, "$replaceRoot tag must not be present when includeTotal is false")

		// Final $project sets total to -1.
		projIdx := findStageIndex(pipeline, "$project")
		require.GreaterOrEqual(t, projIdx, 0, "$project stage not found")
		proj := pipeline[projIdx].(bson.M)["$project"].(bson.M)
		require.EqualValues(t, -1, proj["total"], "total should be -1 when includeTotal is false")
	})

	t.Run("includeTotal=true wires up $replaceRoot + $unionWith + $group + $project", func(t *testing.T) {
		opts := baseCursorOpts()
		opts.filter = bson.M{"status": "active"}
		opts.includeTotal = true

		pipeline := buildCursorPipeline(opts, testCollectionName)

		assertStageOrder(t, pipeline, "$match", "$sort")
		assertStageOrder(t, pipeline, "$sort", "$limit")
		assertStageOrder(t, pipeline, "$limit", "$replaceRoot")
		assertStageOrder(t, pipeline, "$replaceRoot", "$unionWith")
		assertStageOrder(t, pipeline, "$unionWith", "$group")
		assertStageOrder(t, pipeline, "$group", "$project")
	})
}

func TestBuildCursorPipeline_WithStages(t *testing.T) {
	lookupStage := bson.D{{Key: "$lookup", Value: bson.M{"from": "categories", "localField": "category_id", "foreignField": "_id", "as": "category"}}}
	unwindStage := bson.D{{Key: "$unwind", Value: "$category"}}

	opts := baseCursorOpts()
	opts.filter = bson.M{"status": "active"}
	opts.includeTotal = true
	opts.stages = bson.A{lookupStage, unwindStage}

	pipeline := buildCursorPipeline(opts, testCollectionName)

	// Custom stages sit between $sort and $limit.
	assertStageOrder(t, pipeline, "$sort", "$lookup")
	assertStageOrder(t, pipeline, "$lookup", "$unwind")
	assertStageOrder(t, pipeline, "$unwind", "$limit")
}

func TestBuildCursorPipeline_CustomStagesDoNotLeakIntoCountSubPipeline(t *testing.T) {
	// Custom stages like $unwind change cardinality; total must reflect source documents
	// matching the user filter, so custom stages must not run inside the count sub-pipeline.
	unwind := bson.D{{Key: "$unwind", Value: "$tags"}}

	opts := baseCursorOpts()
	opts.filter = bson.M{"status": "active"}
	opts.includeTotal = true
	opts.stages = bson.A{unwind}

	pipeline := buildCursorPipeline(opts, testCollectionName)

	unionIdx := findStageIndex(pipeline, "$unionWith")
	require.GreaterOrEqual(t, unionIdx, 0, "$unionWith stage not found")
	subPipeline := pipeline[unionIdx].(bson.M)["$unionWith"].(bson.M)["pipeline"].(bson.A)

	// Sub-pipeline must contain $match + $count + $replaceRoot only — no $unwind.
	for _, stage := range subPipeline {
		require.Less(t, findStageIndex(bson.A{stage}, "$unwind"), 0, "count sub-pipeline must not include custom $unwind stage")
	}
}

func TestBuildCursorPipeline_CursorFilterDoesNotLeakIntoCountSubPipeline(t *testing.T) {
	// cursor_filter must not affect total: the count sub-pipeline's $match should carry
	// only user filter, never cursor conditions.
	oid := bson.NewObjectID()
	cursor := &Cursor{CursorId: oid.Hex()}

	opts := baseCursorOpts()
	opts.filter = bson.M{"status": "active"}
	opts.cursor = cursor
	opts.includeTotal = true

	pipeline := buildCursorPipeline(opts, testCollectionName)

	// Main pipeline's $match has user_filter AND cursor_filter (combined with $and).
	mainMatch := pipeline[0].(bson.M)["$match"].(bson.M)
	require.Contains(t, mainMatch, "$and", "main $match should combine user and cursor filters")

	// Count sub-pipeline's $match has user_filter only (no $and, no cursor_id).
	unionIdx := findStageIndex(pipeline, "$unionWith")
	require.GreaterOrEqual(t, unionIdx, 0, "$unionWith stage not found")
	subPipeline := pipeline[unionIdx].(bson.M)["$unionWith"].(bson.M)["pipeline"].(bson.A)

	countMatch := subPipeline[0].(bson.M)["$match"].(bson.M)
	require.Equal(t, "active", countMatch["status"], "count sub-pipeline $match should keep user filter")
	require.NotContains(t, countMatch, "$and", "count sub-pipeline $match must not be wrapped in $and (no cursor filter)")
	require.NotContains(t, countMatch, "cursor_id", "count sub-pipeline $match must not contain cursor_id")
}

func TestBuildCursorPipeline_WithDecorationStages(t *testing.T) {
	decoration := bson.D{{Key: "$lookup", Value: bson.M{"from": "translations"}}}

	opts := baseCursorOpts()
	opts.decorationStages = bson.A{decoration}

	pipeline := buildCursorPipeline(opts, testCollectionName)

	// Decoration stage should sit between $limit and the final shaping stages.
	assertStageOrder(t, pipeline, "$limit", "$lookup")
	assertStageOrder(t, pipeline, "$lookup", "$group")

	// $limit must have lookahead applied.
	limitIdx := findStageIndex(pipeline, "$limit")
	require.GreaterOrEqual(t, limitIdx, 0, "$limit stage not found")
	limitVal := pipeline[limitIdx].(bson.M)["$limit"].(int64)
	require.Equal(t, int64(11), limitVal, "$limit should be 10 + 1 lookahead")
}

func TestBuildCursorPipeline_WithBothStageTypes(t *testing.T) {
	stage := bson.D{{Key: "$addFields", Value: bson.M{"computed": true}}}
	decoration := bson.D{{Key: "$lookup", Value: bson.M{"from": "translations"}}}

	opts := baseCursorOpts()
	opts.stages = bson.A{stage}
	opts.decorationStages = bson.A{decoration}

	pipeline := buildCursorPipeline(opts, testCollectionName)

	// Custom stage (opts.stages) comes between $sort and $limit; decoration ($lookup) comes after $limit.
	assertStageOrder(t, pipeline, "$sort", "$addFields")
	assertStageOrder(t, pipeline, "$addFields", "$limit")
	assertStageOrder(t, pipeline, "$limit", "$lookup")
}

func TestBuildCursorPipeline_EmptyStages(t *testing.T) {
	// Verify backward compatibility: nil stages produce identical pipeline to omitting them entirely.
	optsWithout := baseCursorOpts()
	optsWithout.filter = bson.M{"status": "active"}
	optsWithout.includeTotal = true

	optsWith := baseCursorOpts()
	optsWith.filter = bson.M{"status": "active"}
	optsWith.includeTotal = true
	optsWith.stages = nil
	optsWith.decorationStages = nil

	pipelineWithout := buildCursorPipeline(optsWithout, testCollectionName)
	pipelineWith := buildCursorPipeline(optsWith, testCollectionName)

	require.Len(t, pipelineWith, len(pipelineWithout), "pipelines differ in length")
}

func TestStageOptions_Accumulates(t *testing.T) {
	tests := []struct {
		name    string
		apply   func(*listCursorOptions)
		getLen  func(*listCursorOptions) int
		wantLen int
	}{
		{
			name: "WithListCursorStages",
			apply: func(opts *listCursorOptions) {
				WithListCursorStages(bson.D{{Key: "$lookup", Value: bson.M{"from": "a"}}})(opts)
				WithListCursorStages(bson.D{{Key: "$unwind", Value: "$a"}})(opts)
			},
			getLen:  func(opts *listCursorOptions) int { return len(opts.stages) },
			wantLen: 2,
		},
		{
			name: "WithListCursorDecorationStages",
			apply: func(opts *listCursorOptions) {
				WithListCursorDecorationStages(bson.D{{Key: "$lookup", Value: bson.M{"from": "a"}}})(opts)
				WithListCursorDecorationStages(bson.D{{Key: "$lookup", Value: bson.M{"from": "b"}}})(opts)
			},
			getLen:  func(opts *listCursorOptions) int { return len(opts.decorationStages) },
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultListCursorOptions()
			tt.apply(opts)
			require.Equal(t, tt.wantLen, tt.getLen(opts))
		})
	}
}

func TestStageOptions_SkipsNil(t *testing.T) {
	tests := []struct {
		name   string
		apply  func(*listCursorOptions)
		getLen func(*listCursorOptions) int
	}{
		{
			name: "WithListCursorStages",
			apply: func(opts *listCursorOptions) {
				WithListCursorStages(nil, bson.D{{Key: "$lookup", Value: bson.M{"from": "a"}}}, nil)(opts)
			},
			getLen: func(opts *listCursorOptions) int { return len(opts.stages) },
		},
		{
			name: "WithListCursorDecorationStages",
			apply: func(opts *listCursorOptions) {
				WithListCursorDecorationStages(nil, bson.D{{Key: "$lookup", Value: bson.M{"from": "a"}}}, nil)(opts)
			},
			getLen: func(opts *listCursorOptions) int { return len(opts.decorationStages) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultListCursorOptions()
			tt.apply(opts)
			require.Equal(t, 1, tt.getLen(opts), "nil should be filtered")
		})
	}
}

// assertStageOrder verifies that stage at key a appears before stage at key b in the pipeline.
func assertStageOrder(t *testing.T, pipeline bson.A, before, after string) {
	t.Helper()
	bi := findStageIndex(pipeline, before)
	ai := findStageIndex(pipeline, after)
	require.GreaterOrEqual(t, bi, 0, "stage %q not found in pipeline", before)
	require.GreaterOrEqual(t, ai, 0, "stage %q not found in pipeline", after)
	require.Less(t, bi, ai, "%s (idx %d) should be before %s (idx %d)", before, bi, after, ai)
}

// baseCursorOpts returns a minimal listCursorOptions for testing.
func baseCursorOpts() *listCursorOptions {
	return &listCursorOptions{
		sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
		filter:        bson.M{},
		limit:         10,
		cursorIdField: "cursor_id",
	}
}

// findStageIndex returns the index of the first pipeline stage with the given key, or -1.
func findStageIndex(pipeline bson.A, key string) int {
	for i, stage := range pipeline {
		if stageMap, ok := stage.(bson.M); ok {
			if _, ok := stageMap[key]; ok {
				return i
			}
		}
		if stageDoc, ok := stage.(bson.D); ok {
			for _, e := range stageDoc {
				if e.Key == key {
					return i
				}
			}
		}
	}
	return -1
}

// assertBsonDStageKey asserts that a pipeline stage (bson.M or bson.D) contains the given key.
func assertBsonDStageKey(t *testing.T, stage any, key string) {
	t.Helper()
	switch s := stage.(type) {
	case bson.M:
		require.Contains(t, s, key, "expected stage key %q", key)
	case bson.D:
		for _, e := range s {
			if e.Key == key {
				return
			}
		}
		require.Fail(t, "expected stage key %q, got %v", key, s)
	default:
		require.Fail(t, fmt.Sprintf("unexpected stage type %T", stage))
	}
}

// assertBsonDEqual compares two bson.D values element by element.
func assertBsonDEqual(t *testing.T, want, got bson.D) {
	t.Helper()
	require.Len(t, got, len(want), "bson.D len mismatch\n  got:  %v\n  want: %v", got, want)
	for i := range want {
		require.Equal(t, want[i].Key, got[i].Key, "[%d] key mismatch", i)
		require.Equal(t, fmt.Sprint(want[i].Value), fmt.Sprint(got[i].Value), "[%d] value mismatch", i)
	}
}

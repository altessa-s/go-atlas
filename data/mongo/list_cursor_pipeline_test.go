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

func TestBuildFacetStage(t *testing.T) {
	t.Run("without total and without projection", func(t *testing.T) {
		got := buildFacetStage(10, nil, false, nil, false)
		require.Len(t, got, 1)
		facet := got[0].(bson.M)["$facet"].(bson.M)
		require.NotContains(t, facet, "count", "count should not be present when includeTotal is false")
		items := facet["items"].(bson.A)
		limitStage := items[0].(bson.M)
		require.Equal(t, int64(11), limitStage["$limit"], "limit should be 10 + lookahead")
	})

	t.Run("with total", func(t *testing.T) {
		got := buildFacetStage(20, nil, true, nil, false)
		facet := got[0].(bson.M)["$facet"].(bson.M)
		require.Contains(t, facet, "count", "count should be present when includeTotal is true")
	})

	t.Run("with projection", func(t *testing.T) {
		proj := bson.M{"name": 1, "age": 1}
		got := buildFacetStage(10, proj, false, nil, false)
		facet := got[0].(bson.M)["$facet"].(bson.M)
		items := facet["items"].(bson.A)
		require.Len(t, items, 2, "expected $limit + $project")
	})
}

func TestBuildCursorPipeline(t *testing.T) {
	t.Run("basic pipeline has correct stage order", func(t *testing.T) {
		opts := &listCursorOptions{
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
			filter:        bson.M{"status": "active"},
			limit:         10,
			cursorIdField: "cursor_id",
			includeTotal:  true,
		}

		pipeline := buildCursorPipeline(opts)

		// Pipeline should have: $match, $sort, $facet, $unwind, $project (at minimum)
		require.GreaterOrEqual(t, len(pipeline), 3)

		// First stage must be $match
		require.Contains(t, pipeline[0].(bson.M), "$match", "first stage should be $match")

		// Second stage must be $sort
		sortStage, ok := pipeline[1].(bson.M)["$sort"]
		require.True(t, ok, "second stage should be $sort")

		// Sort must include cursor_id tiebreaker
		sortDoc := sortStage.(bson.D)
		lastField := sortDoc[len(sortDoc)-1]
		require.Equal(t, "cursor_id", lastField.Key, "last sort field")
	})

	t.Run("opts.sort is not mutated by tiebreaker addition", func(t *testing.T) {
		opts := &listCursorOptions{
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
			filter:        bson.M{},
			limit:         10,
			cursorIdField: "cursor_id",
		}
		originalSortLen := len(opts.sort)

		buildCursorPipeline(opts)

		require.Len(t, opts.sort, originalSortLen, "opts.sort was mutated")
	})

	t.Run("sort already containing cursor_id is not modified", func(t *testing.T) {
		opts := &listCursorOptions{
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}, {Key: "cursor_id", Value: int32(-1)}},
			filter:        bson.M{},
			limit:         10,
			cursorIdField: "cursor_id",
		}

		pipeline := buildCursorPipeline(opts)

		sortStage := pipeline[0].(bson.M)["$sort"].(bson.D) // $sort is first when no filter/cursor
		require.Len(t, sortStage, 2, "sort should still have 2 fields")
	})

	t.Run("without total omits count in facet", func(t *testing.T) {
		opts := baseCursorOpts()

		pipeline := buildCursorPipeline(opts)

		facetIdx := findStageIndex(pipeline, "$facet")
		require.GreaterOrEqual(t, facetIdx, 0, "$facet stage not found")
		facet := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
		require.NotContains(t, facet, "count", "facet should not include count when includeTotal is false")
	})
}

func TestBuildFacetStage_WithDecorationStages(t *testing.T) {
	decoration := bson.A{
		bson.D{{Key: "$lookup", Value: bson.M{"from": "translations", "localField": "_id", "foreignField": "entity_id", "as": "translations"}}},
	}

	got := buildFacetStage(10, nil, false, decoration, false)
	facet := got[0].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)

	// items pipeline: $limit, $lookup
	require.Len(t, items, 2)

	assertBsonDStageKey(t, items[0], "$limit")
	assertBsonDStageKey(t, items[1], "$lookup")
}

func TestBuildFacetStage_WithDecorationAndProjection(t *testing.T) {
	decoration := bson.A{
		bson.D{{Key: "$lookup", Value: bson.M{"from": "translations"}}},
	}
	proj := bson.M{"name": 1}

	got := buildFacetStage(10, proj, false, decoration, false)
	facet := got[0].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)

	// items pipeline: $limit, $lookup, $project
	require.Len(t, items, 3)

	assertBsonDStageKey(t, items[0], "$limit")
	assertBsonDStageKey(t, items[1], "$lookup")
	assertBsonDStageKey(t, items[2], "$project")
}

func TestBuildCursorPipeline_WithStages(t *testing.T) {
	lookupStage := bson.D{{Key: "$lookup", Value: bson.M{"from": "categories", "localField": "category_id", "foreignField": "_id", "as": "category"}}}
	unwindStage := bson.D{{Key: "$unwind", Value: "$category"}}

	opts := baseCursorOpts()
	opts.filter = bson.M{"status": "active"}
	opts.stages = bson.A{lookupStage, unwindStage}

	pipeline := buildCursorPipeline(opts)

	assertStageOrder(t, pipeline, "$sort", "$lookup")
	assertStageOrder(t, pipeline, "$lookup", "$unwind")
	assertStageOrder(t, pipeline, "$unwind", "$facet")
}

func TestBuildCursorPipeline_WithDecorationStages(t *testing.T) {
	decoration := bson.D{{Key: "$lookup", Value: bson.M{"from": "translations"}}}

	opts := baseCursorOpts()
	opts.decorationStages = bson.A{decoration}

	pipeline := buildCursorPipeline(opts)

	facetIdx := findStageIndex(pipeline, "$facet")
	require.GreaterOrEqual(t, facetIdx, 0, "$facet stage not found")

	facet := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)

	require.GreaterOrEqual(t, len(items), 2)

	assertBsonDStageKey(t, items[0], "$limit")
	assertBsonDStageKey(t, items[1], "$lookup")

	// Verify $limit has lookahead
	limitVal := items[0].(bson.M)["$limit"].(int64)
	require.Equal(t, int64(11), limitVal, "$limit should be 10 + 1 lookahead")
}

func TestBuildCursorPipeline_WithBothStageTypes(t *testing.T) {
	stage := bson.D{{Key: "$addFields", Value: bson.M{"computed": true}}}
	decoration := bson.D{{Key: "$lookup", Value: bson.M{"from": "translations"}}}

	opts := baseCursorOpts()
	opts.stages = bson.A{stage}
	opts.decorationStages = bson.A{decoration}

	pipeline := buildCursorPipeline(opts)

	assertStageOrder(t, pipeline, "$sort", "$addFields")
	assertStageOrder(t, pipeline, "$addFields", "$facet")

	// Decoration stage should be in facet items pipeline
	facetIdx := findStageIndex(pipeline, "$facet")
	facet := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)

	assertBsonDStageKey(t, items[0], "$limit")
	assertBsonDStageKey(t, items[1], "$lookup")
}

func TestBuildCursorPipeline_EmptyStages(t *testing.T) {
	// Verify backward compatibility: nil stages produce identical pipeline
	optsWithout := baseCursorOpts()
	optsWithout.filter = bson.M{"status": "active"}
	optsWithout.includeTotal = true

	optsWith := baseCursorOpts()
	optsWith.filter = bson.M{"status": "active"}
	optsWith.includeTotal = true
	optsWith.stages = nil
	optsWith.decorationStages = nil

	pipelineWithout := buildCursorPipeline(optsWithout)
	pipelineWith := buildCursorPipeline(optsWith)

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

func TestBuildFacetStage_PreCountedTotal(t *testing.T) {
	t.Run("preCountedTotal adds $unset and reads from annotation field", func(t *testing.T) {
		got := buildFacetStage(10, nil, true, nil, true)
		facet := got[0].(bson.M)["$facet"].(bson.M)

		// Items pipeline should have: $limit, $unset
		items := facet["items"].(bson.A)
		require.Len(t, items, 2)
		assertBsonDStageKey(t, items[0], "$limit")
		assertBsonDStageKey(t, items[1], "$unset")

		// Count branch should read from annotation field, not use $count
		count := facet["count"].(bson.A)
		require.Len(t, count, 2)
		assertBsonDStageKey(t, count[0], "$limit")
		assertBsonDStageKey(t, count[1], "$project")
	})

	t.Run("preCountedTotal false uses standard $count", func(t *testing.T) {
		got := buildFacetStage(10, nil, true, nil, false)
		facet := got[0].(bson.M)["$facet"].(bson.M)

		count := facet["count"].(bson.A)
		require.Len(t, count, 1)
		assertBsonDStageKey(t, count[0], "$count")
	})
}

func TestBuildCursorPipeline_PreCountedTotalWithStages(t *testing.T) {
	t.Run("stages + includeTotal injects $setWindowFields", func(t *testing.T) {
		opts := baseCursorOpts()
		opts.includeTotal = true
		opts.stages = bson.A{bson.D{{Key: "$unwind", Value: "$tags"}}}

		pipeline := buildCursorPipeline(opts)

		assertStageOrder(t, pipeline, "$setWindowFields", "$unwind")
		assertStageOrder(t, pipeline, "$unwind", "$facet")

		// Facet count branch should use pre-computed total
		facetIdx := findStageIndex(pipeline, "$facet")
		facet := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
		count := facet["count"].(bson.A)
		assertBsonDStageKey(t, count[0], "$limit")
		assertBsonDStageKey(t, count[1], "$project")
	})

	t.Run("stages without includeTotal skips $setWindowFields", func(t *testing.T) {
		opts := baseCursorOpts()
		opts.stages = bson.A{bson.D{{Key: "$unwind", Value: "$tags"}}}

		pipeline := buildCursorPipeline(opts)

		require.Less(t, findStageIndex(pipeline, "$setWindowFields"), 0, "$setWindowFields should not be present when includeTotal is false")
	})

	t.Run("includeTotal without stages uses standard $count", func(t *testing.T) {
		opts := baseCursorOpts()
		opts.includeTotal = true

		pipeline := buildCursorPipeline(opts)

		require.Less(t, findStageIndex(pipeline, "$setWindowFields"), 0, "$setWindowFields should not be present when no custom stages")

		facetIdx := findStageIndex(pipeline, "$facet")
		facet := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
		count := facet["count"].(bson.A)
		assertBsonDStageKey(t, count[0], "$count")
	})
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

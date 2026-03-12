// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"fmt"
	"testing"

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

			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d; got %v", len(got), tt.wantLen, got)
			}

			if tt.wantSameSlice {
				if &got[0] != &tt.sort[0] {
					t.Error("expected same underlying slice, got a copy")
				}
				return
			}

			last := got[len(got)-1]
			if last.Key != tt.wantLastKey {
				t.Errorf("last key = %q, want %q", last.Key, tt.wantLastKey)
			}
			if last.Value != tt.wantLastVal {
				t.Errorf("last value = %v (%T), want %v (%T)", last.Value, last.Value, tt.wantLastVal, tt.wantLastVal)
			}
		})
	}
}

func TestEnsureCursorIdInSort_DoesNotMutateOriginal(t *testing.T) {
	original := bson.D{{Key: "created_at", Value: int32(-1)}}
	originalCopy := make(bson.D, len(original))
	copy(originalCopy, original)

	result := ensureCursorIdInSort(original, "cursor_id")

	// Original must be unchanged
	if len(original) != len(originalCopy) {
		t.Fatalf("original length changed: %d → %d", len(originalCopy), len(original))
	}
	for i := range original {
		if original[i].Key != originalCopy[i].Key || original[i].Value != originalCopy[i].Value {
			t.Errorf("original[%d] mutated: got {%s, %v}, want {%s, %v}",
				i, original[i].Key, original[i].Value, originalCopy[i].Key, originalCopy[i].Value)
		}
	}

	// Result must be a different slice
	if len(result) != 2 {
		t.Fatalf("result len = %d, want 2", len(result))
	}
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
			if len(got) != 1 {
				t.Fatalf("expected 1 stage, got %d", len(got))
			}
			stage, ok := got[0].(bson.M)
			if !ok {
				t.Fatal("stage is not bson.M")
			}
			sortSpec, ok := stage["$sort"].(bson.D)
			if !ok {
				t.Fatal("$sort value is not bson.D")
			}
			assertBsonDEqual(t, tt.wantSort, sortSpec)
		})
	}
}

func TestBuildMatchStage(t *testing.T) {
	t.Run("no filter no cursor", func(t *testing.T) {
		got := buildMatchStage(bson.M{}, nil, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		if len(got) != 0 {
			t.Fatalf("expected empty pipeline, got %d stages", len(got))
		}
	})

	t.Run("filter only", func(t *testing.T) {
		filter := bson.M{"status": "active"}
		got := buildMatchStage(filter, nil, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		if len(got) != 1 {
			t.Fatalf("expected 1 stage, got %d", len(got))
		}
		stage := got[0].(bson.M)
		match := stage["$match"].(bson.M)
		if match["status"] != "active" {
			t.Errorf("filter not applied: %v", match)
		}
	})

	t.Run("cursor only", func(t *testing.T) {
		oid := bson.NewObjectID()
		cursor := &Cursor{CursorId: oid.Hex()}
		got := buildMatchStage(bson.M{}, cursor, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		if len(got) != 1 {
			t.Fatalf("expected 1 stage, got %d", len(got))
		}
		stage := got[0].(bson.M)
		match := stage["$match"].(bson.M)
		if _, ok := match["cursor_id"]; !ok {
			t.Error("cursor filter not applied")
		}
	})

	t.Run("filter and cursor combined with $and", func(t *testing.T) {
		oid := bson.NewObjectID()
		cursor := &Cursor{CursorId: oid.Hex()}
		filter := bson.M{"status": "active"}
		got := buildMatchStage(filter, cursor, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		if len(got) != 1 {
			t.Fatalf("expected 1 stage, got %d", len(got))
		}
		stage := got[0].(bson.M)
		match := stage["$match"].(bson.M)
		if _, ok := match["$and"]; !ok {
			t.Error("expected $and operator for combined filter")
		}
	})

	t.Run("zero cursor treated as no cursor", func(t *testing.T) {
		cursor := &Cursor{} // zero cursor
		got := buildMatchStage(bson.M{}, cursor, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		if len(got) != 0 {
			t.Fatalf("expected empty pipeline for zero cursor, got %d stages", len(got))
		}
	})
}

func TestBuildCursorFilter(t *testing.T) {
	oid := bson.NewObjectID()

	t.Run("nil cursor returns empty", func(t *testing.T) {
		got := buildCursorFilter(nil, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		if len(got) != 0 {
			t.Fatalf("expected empty filter, got %v", got)
		}
	})

	t.Run("zero cursor returns empty", func(t *testing.T) {
		got := buildCursorFilter(&Cursor{}, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		if len(got) != 0 {
			t.Fatalf("expected empty filter, got %v", got)
		}
	})

	t.Run("descending sort uses $lt", func(t *testing.T) {
		cursor := &Cursor{CursorId: oid.Hex()}
		got := buildCursorFilter(cursor, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		cursorFilter, ok := got["cursor_id"].(bson.M)
		if !ok {
			t.Fatalf("expected cursor_id filter, got %v", got)
		}
		if _, ok := cursorFilter["$lt"]; !ok {
			t.Errorf("expected $lt for descending sort, got %v", cursorFilter)
		}
	})

	t.Run("ascending sort uses $gt", func(t *testing.T) {
		cursor := &Cursor{CursorId: oid.Hex()}
		got := buildCursorFilter(cursor, "cursor_id", bson.D{{Key: "created_at", Value: 1}})
		cursorFilter, ok := got["cursor_id"].(bson.M)
		if !ok {
			t.Fatalf("expected cursor_id filter, got %v", got)
		}
		if _, ok := cursorFilter["$gt"]; !ok {
			t.Errorf("expected $gt for ascending sort, got %v", cursorFilter)
		}
	})

	t.Run("with sort value uses compound $or filter", func(t *testing.T) {
		sortVal, err := encodeSortValue(int64(1234567890))
		if err != nil {
			t.Fatalf("encodeSortValue: %v", err)
		}
		cursor := &Cursor{CursorId: oid.Hex(), SortValue: sortVal}
		got := buildCursorFilter(cursor, "cursor_id", bson.D{{Key: "updated_at", Value: -1}})
		if _, ok := got["$or"]; !ok {
			t.Errorf("expected $or filter for compound sort, got %v", got)
		}
	})

	t.Run("invalid cursor id returns empty", func(t *testing.T) {
		cursor := &Cursor{CursorId: "invalid-hex"}
		got := buildCursorFilter(cursor, "cursor_id", bson.D{{Key: "created_at", Value: -1}})
		if len(got) != 0 {
			t.Fatalf("expected empty filter for invalid cursor id, got %v", got)
		}
	})
}

func TestBuildFacetStage(t *testing.T) {
	t.Run("without total and without projection", func(t *testing.T) {
		got := buildFacetStage(10, nil, false, nil, false)
		if len(got) != 1 {
			t.Fatalf("expected 1 stage, got %d", len(got))
		}
		facet := got[0].(bson.M)["$facet"].(bson.M)
		if _, ok := facet["count"]; ok {
			t.Error("count should not be present when includeTotal is false")
		}
		items := facet["items"].(bson.A)
		limitStage := items[0].(bson.M)
		if limitStage["$limit"] != int64(11) { // 10 + lookahead
			t.Errorf("limit = %v, want 11", limitStage["$limit"])
		}
	})

	t.Run("with total", func(t *testing.T) {
		got := buildFacetStage(20, nil, true, nil, false)
		facet := got[0].(bson.M)["$facet"].(bson.M)
		if _, ok := facet["count"]; !ok {
			t.Error("count should be present when includeTotal is true")
		}
	})

	t.Run("with projection", func(t *testing.T) {
		proj := bson.M{"name": 1, "age": 1}
		got := buildFacetStage(10, proj, false, nil, false)
		facet := got[0].(bson.M)["$facet"].(bson.M)
		items := facet["items"].(bson.A)
		if len(items) != 2 { // $limit + $project
			t.Fatalf("expected 2 stages in items pipeline, got %d", len(items))
		}
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
		if len(pipeline) < 3 {
			t.Fatalf("expected at least 3 stages, got %d", len(pipeline))
		}

		// First stage must be $match
		if _, ok := pipeline[0].(bson.M)["$match"]; !ok {
			t.Error("first stage should be $match")
		}

		// Second stage must be $sort
		sortStage, ok := pipeline[1].(bson.M)["$sort"]
		if !ok {
			t.Fatal("second stage should be $sort")
		}

		// Sort must include cursor_id tiebreaker
		sortDoc := sortStage.(bson.D)
		lastField := sortDoc[len(sortDoc)-1]
		if lastField.Key != "cursor_id" {
			t.Errorf("last sort field = %q, want cursor_id", lastField.Key)
		}
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

		if len(opts.sort) != originalSortLen {
			t.Errorf("opts.sort was mutated: len changed from %d to %d", originalSortLen, len(opts.sort))
		}
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
		if len(sortStage) != 2 {
			t.Errorf("sort should still have 2 fields, got %d", len(sortStage))
		}
	})

	t.Run("without total omits count in facet", func(t *testing.T) {
		opts := &listCursorOptions{
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
			filter:        bson.M{},
			limit:         10,
			cursorIdField: "cursor_id",
			includeTotal:  false,
		}

		pipeline := buildCursorPipeline(opts)

		// Find facet stage
		for _, stage := range pipeline {
			if stageMap, ok := stage.(bson.M); ok {
				if facet, ok := stageMap["$facet"].(bson.M); ok {
					if _, hasCount := facet["count"]; hasCount {
						t.Error("facet should not include count when includeTotal is false")
					}
					return
				}
			}
		}
		t.Fatal("$facet stage not found")
	})
}

func TestBuildFacetStage_WithDecorationStages(t *testing.T) {
	decoration := bson.A{
		bson.D{{"$lookup", bson.M{"from": "translations", "localField": "_id", "foreignField": "entity_id", "as": "translations"}}},
	}

	got := buildFacetStage(10, nil, false, decoration, false)
	facet := got[0].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)

	// items pipeline: $limit, $lookup
	if len(items) != 2 {
		t.Fatalf("expected 2 stages in items pipeline, got %d", len(items))
	}

	assertBsonDStageKey(t, items[0], "$limit")
	assertBsonDStageKey(t, items[1], "$lookup")
}

func TestBuildFacetStage_WithDecorationAndProjection(t *testing.T) {
	decoration := bson.A{
		bson.D{{"$lookup", bson.M{"from": "translations"}}},
	}
	proj := bson.M{"name": 1}

	got := buildFacetStage(10, proj, false, decoration, false)
	facet := got[0].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)

	// items pipeline: $limit, $lookup, $project
	if len(items) != 3 {
		t.Fatalf("expected 3 stages in items pipeline, got %d", len(items))
	}

	assertBsonDStageKey(t, items[0], "$limit")
	assertBsonDStageKey(t, items[1], "$lookup")
	assertBsonDStageKey(t, items[2], "$project")
}

func TestBuildCursorPipeline_WithStages(t *testing.T) {
	lookupStage := bson.D{{"$lookup", bson.M{"from": "categories", "localField": "category_id", "foreignField": "_id", "as": "category"}}}
	unwindStage := bson.D{{"$unwind", "$category"}}

	opts := &listCursorOptions{
		sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
		filter:        bson.M{"status": "active"},
		limit:         10,
		cursorIdField: "cursor_id",
		stages:        bson.A{lookupStage, unwindStage},
	}

	pipeline := buildCursorPipeline(opts)

	// Find positions of key stages
	sortIdx := findStageIndex(pipeline, "$sort")
	lookupIdx := findStageIndex(pipeline, "$lookup")
	unwindIdx := findStageIndex(pipeline, "$unwind")
	facetIdx := findStageIndex(pipeline, "$facet")

	if sortIdx < 0 || lookupIdx < 0 || unwindIdx < 0 || facetIdx < 0 {
		t.Fatalf("missing expected stages: sort=%d, lookup=%d, unwind=%d, facet=%d",
			sortIdx, lookupIdx, unwindIdx, facetIdx)
	}

	// Custom stages must be after $sort and before $facet
	if lookupIdx <= sortIdx {
		t.Errorf("$lookup (idx %d) should be after $sort (idx %d)", lookupIdx, sortIdx)
	}
	if unwindIdx <= lookupIdx {
		t.Errorf("$unwind (idx %d) should be after $lookup (idx %d)", unwindIdx, lookupIdx)
	}
	if facetIdx <= unwindIdx {
		t.Errorf("$facet (idx %d) should be after $unwind (idx %d)", facetIdx, unwindIdx)
	}
}

func TestBuildCursorPipeline_WithDecorationStages(t *testing.T) {
	decoration := bson.D{{"$lookup", bson.M{"from": "translations"}}}

	opts := &listCursorOptions{
		sort:             bson.D{{Key: "created_at", Value: int32(-1)}},
		filter:           bson.M{},
		limit:            10,
		cursorIdField:    "cursor_id",
		decorationStages: bson.A{decoration},
	}

	pipeline := buildCursorPipeline(opts)

	// Find facet stage and check items pipeline
	facetIdx := findStageIndex(pipeline, "$facet")
	if facetIdx < 0 {
		t.Fatal("$facet stage not found")
	}

	facet := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)

	// items: $limit, $lookup
	if len(items) < 2 {
		t.Fatalf("expected at least 2 stages in items pipeline, got %d", len(items))
	}

	assertBsonDStageKey(t, items[0], "$limit")
	assertBsonDStageKey(t, items[1], "$lookup")

	// Verify $limit has lookahead
	limitVal := items[0].(bson.M)["$limit"].(int64)
	if limitVal != 11 { // 10 + 1 lookahead
		t.Errorf("$limit = %d, want 11", limitVal)
	}
}

func TestBuildCursorPipeline_WithBothStageTypes(t *testing.T) {
	stage := bson.D{{"$addFields", bson.M{"computed": true}}}
	decoration := bson.D{{"$lookup", bson.M{"from": "translations"}}}

	opts := &listCursorOptions{
		sort:             bson.D{{Key: "created_at", Value: int32(-1)}},
		filter:           bson.M{},
		limit:            10,
		cursorIdField:    "cursor_id",
		stages:           bson.A{stage},
		decorationStages: bson.A{decoration},
	}

	pipeline := buildCursorPipeline(opts)

	// Custom stage should be in main pipeline after $sort
	sortIdx := findStageIndex(pipeline, "$sort")
	addFieldsIdx := findStageIndex(pipeline, "$addFields")
	facetIdx := findStageIndex(pipeline, "$facet")

	if addFieldsIdx <= sortIdx {
		t.Errorf("$addFields (idx %d) should be after $sort (idx %d)", addFieldsIdx, sortIdx)
	}
	if facetIdx <= addFieldsIdx {
		t.Errorf("$facet (idx %d) should be after $addFields (idx %d)", facetIdx, addFieldsIdx)
	}

	// Decoration stage should be in facet items pipeline
	facet := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)

	assertBsonDStageKey(t, items[0], "$limit")
	assertBsonDStageKey(t, items[1], "$lookup")
}

func TestBuildCursorPipeline_EmptyStages(t *testing.T) {
	// Verify backward compatibility: nil stages produce identical pipeline
	optsWithout := &listCursorOptions{
		sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
		filter:        bson.M{"status": "active"},
		limit:         10,
		cursorIdField: "cursor_id",
		includeTotal:  true,
	}

	optsWith := &listCursorOptions{
		sort:             bson.D{{Key: "created_at", Value: int32(-1)}},
		filter:           bson.M{"status": "active"},
		limit:            10,
		cursorIdField:    "cursor_id",
		includeTotal:     true,
		stages:           nil,
		decorationStages: nil,
	}

	pipelineWithout := buildCursorPipeline(optsWithout)
	pipelineWith := buildCursorPipeline(optsWith)

	if len(pipelineWithout) != len(pipelineWith) {
		t.Fatalf("pipelines differ in length: %d vs %d", len(pipelineWithout), len(pipelineWith))
	}
}

func TestWithListCursorStages_Accumulates(t *testing.T) {
	opts := defaultListCursorOptions()

	opt1 := WithListCursorStages(bson.D{{"$lookup", bson.M{"from": "a"}}})
	opt2 := WithListCursorStages(bson.D{{"$unwind", "$a"}})

	opt1(opts)
	opt2(opts)

	if len(opts.stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(opts.stages))
	}
}

func TestWithListCursorStages_SkipsNil(t *testing.T) {
	opts := defaultListCursorOptions()

	opt := WithListCursorStages(nil, bson.D{{"$lookup", bson.M{"from": "a"}}}, nil)
	opt(opts)

	if len(opts.stages) != 1 {
		t.Fatalf("expected 1 stage (nil filtered), got %d", len(opts.stages))
	}
}

func TestWithListCursorDecorationStages_Accumulates(t *testing.T) {
	opts := defaultListCursorOptions()

	opt1 := WithListCursorDecorationStages(bson.D{{"$lookup", bson.M{"from": "a"}}})
	opt2 := WithListCursorDecorationStages(bson.D{{"$lookup", bson.M{"from": "b"}}})

	opt1(opts)
	opt2(opts)

	if len(opts.decorationStages) != 2 {
		t.Fatalf("expected 2 decoration stages, got %d", len(opts.decorationStages))
	}
}

func TestBuildFacetStage_PreCountedTotal(t *testing.T) {
	t.Run("preCountedTotal adds $unset and reads from annotation field", func(t *testing.T) {
		got := buildFacetStage(10, nil, true, nil, true)
		facet := got[0].(bson.M)["$facet"].(bson.M)

		// Items pipeline should have: $limit, $unset
		items := facet["items"].(bson.A)
		if len(items) != 2 {
			t.Fatalf("expected 2 stages in items pipeline, got %d", len(items))
		}
		assertBsonDStageKey(t, items[0], "$limit")
		assertBsonDStageKey(t, items[1], "$unset")

		// Count branch should read from annotation field, not use $count
		count := facet["count"].(bson.A)
		if len(count) != 2 {
			t.Fatalf("expected 2 stages in count pipeline, got %d", len(count))
		}
		assertBsonDStageKey(t, count[0], "$limit")
		assertBsonDStageKey(t, count[1], "$project")
	})

	t.Run("preCountedTotal false uses standard $count", func(t *testing.T) {
		got := buildFacetStage(10, nil, true, nil, false)
		facet := got[0].(bson.M)["$facet"].(bson.M)

		count := facet["count"].(bson.A)
		if len(count) != 1 {
			t.Fatalf("expected 1 stage in count pipeline, got %d", len(count))
		}
		assertBsonDStageKey(t, count[0], "$count")
	})
}

func TestBuildCursorPipeline_PreCountedTotalWithStages(t *testing.T) {
	t.Run("stages + includeTotal injects $setWindowFields", func(t *testing.T) {
		opts := &listCursorOptions{
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
			filter:        bson.M{},
			limit:         10,
			cursorIdField: "cursor_id",
			includeTotal:  true,
			stages:        bson.A{bson.D{{"$unwind", "$tags"}}},
		}

		pipeline := buildCursorPipeline(opts)

		swfIdx := findStageIndex(pipeline, "$setWindowFields")
		unwindIdx := findStageIndex(pipeline, "$unwind")
		facetIdx := findStageIndex(pipeline, "$facet")

		if swfIdx < 0 {
			t.Fatal("$setWindowFields stage not found")
		}
		if swfIdx >= unwindIdx {
			t.Errorf("$setWindowFields (idx %d) should be before $unwind (idx %d)", swfIdx, unwindIdx)
		}
		if unwindIdx >= facetIdx {
			t.Errorf("$unwind (idx %d) should be before $facet (idx %d)", unwindIdx, facetIdx)
		}

		// Facet count branch should use pre-computed total
		facet := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
		count := facet["count"].(bson.A)
		assertBsonDStageKey(t, count[0], "$limit")
		assertBsonDStageKey(t, count[1], "$project")
	})

	t.Run("stages without includeTotal skips $setWindowFields", func(t *testing.T) {
		opts := &listCursorOptions{
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
			filter:        bson.M{},
			limit:         10,
			cursorIdField: "cursor_id",
			includeTotal:  false,
			stages:        bson.A{bson.D{{"$unwind", "$tags"}}},
		}

		pipeline := buildCursorPipeline(opts)

		if findStageIndex(pipeline, "$setWindowFields") >= 0 {
			t.Error("$setWindowFields should not be present when includeTotal is false")
		}
	})

	t.Run("includeTotal without stages uses standard $count", func(t *testing.T) {
		opts := &listCursorOptions{
			sort:          bson.D{{Key: "created_at", Value: int32(-1)}},
			filter:        bson.M{},
			limit:         10,
			cursorIdField: "cursor_id",
			includeTotal:  true,
		}

		pipeline := buildCursorPipeline(opts)

		if findStageIndex(pipeline, "$setWindowFields") >= 0 {
			t.Error("$setWindowFields should not be present when no custom stages")
		}

		facetIdx := findStageIndex(pipeline, "$facet")
		facet := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
		count := facet["count"].(bson.A)
		assertBsonDStageKey(t, count[0], "$count")
	})
}

func TestWithListCursorDecorationStages_SkipsNil(t *testing.T) {
	opts := defaultListCursorOptions()

	opt := WithListCursorDecorationStages(nil, bson.D{{"$lookup", bson.M{"from": "a"}}}, nil)
	opt(opts)

	if len(opts.decorationStages) != 1 {
		t.Fatalf("expected 1 decoration stage (nil filtered), got %d", len(opts.decorationStages))
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
		if _, ok := s[key]; !ok {
			t.Errorf("expected stage key %q, got %v", key, s)
		}
	case bson.D:
		for _, e := range s {
			if e.Key == key {
				return
			}
		}
		t.Errorf("expected stage key %q, got %v", key, s)
	default:
		t.Errorf("unexpected stage type %T", stage)
	}
}

// assertBsonDEqual compares two bson.D values element by element.
func assertBsonDEqual(t *testing.T, want, got bson.D) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("bson.D len: got %d, want %d\n  got:  %v\n  want: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if want[i].Key != got[i].Key {
			t.Errorf("[%d] key: got %q, want %q", i, got[i].Key, want[i].Key)
		}
		if fmt.Sprint(want[i].Value) != fmt.Sprint(got[i].Value) {
			t.Errorf("[%d] value: got %v, want %v", i, got[i].Value, want[i].Value)
		}
	}
}

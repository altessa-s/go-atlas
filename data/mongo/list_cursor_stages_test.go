package mongo

import (
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestBuildCursorPipeline_WithStages(t *testing.T) {
	lookupStage := bson.D{{"$lookup", bson.M{
		"from":         "categories",
		"localField":   "category_id",
		"foreignField": "_id",
		"as":           "_category",
	}}}

	opts := defaultListCursorOptions()
	WithListCursorStages(lookupStage)(opts)

	pipeline := buildCursorPipeline(opts)

	// Pipeline: $sort → $lookup → $facet → $project
	sortIdx := findStageIndex(pipeline, "$sort")
	lookupIdx := findStageIndex(pipeline, "$lookup")
	facetIdx := findStageIndex(pipeline, "$facet")

	if sortIdx == -1 {
		t.Fatal("$sort stage not found in pipeline")
	}
	if lookupIdx == -1 {
		t.Fatal("$lookup stage not found in pipeline")
	}
	if facetIdx == -1 {
		t.Fatal("$facet stage not found in pipeline")
	}

	if lookupIdx <= sortIdx {
		t.Errorf("$lookup (idx=%d) should come after $sort (idx=%d)", lookupIdx, sortIdx)
	}
	if lookupIdx >= facetIdx {
		t.Errorf("$lookup (idx=%d) should come before $facet (idx=%d)", lookupIdx, facetIdx)
	}
}

func TestBuildCursorPipeline_WithMultipleStages(t *testing.T) {
	lookup := bson.D{{"$lookup", bson.M{
		"from": "categories",
		"as":   "_category",
	}}}
	addFields := bson.D{{"$addFields", bson.M{
		"category_name": "$_category.name",
	}}}

	opts := defaultListCursorOptions()
	WithListCursorStages(lookup, addFields)(opts)

	pipeline := buildCursorPipeline(opts)

	lookupIdx := findStageIndex(pipeline, "$lookup")
	addFieldsIdx := findStageIndex(pipeline, "$addFields")
	facetIdx := findStageIndex(pipeline, "$facet")

	if lookupIdx == -1 || addFieldsIdx == -1 {
		t.Fatal("expected both $lookup and $addFields in pipeline")
	}
	if addFieldsIdx != lookupIdx+1 {
		t.Errorf("$addFields (idx=%d) should immediately follow $lookup (idx=%d)", addFieldsIdx, lookupIdx)
	}
	if addFieldsIdx >= facetIdx {
		t.Errorf("$addFields (idx=%d) should come before $facet (idx=%d)", addFieldsIdx, facetIdx)
	}
}

func TestBuildCursorPipeline_WithDecorationStages(t *testing.T) {
	translationLookup := bson.D{{"$lookup", bson.M{
		"from":         "translations",
		"localField":   "code",
		"foreignField": "code",
		"as":           "_translations",
	}}}

	opts := defaultListCursorOptions()
	WithListCursorDecorationStages(translationLookup)(opts)

	pipeline := buildCursorPipeline(opts)

	// Decoration stages should be inside $facet → items branch, after $limit
	facetIdx := findStageIndex(pipeline, "$facet")
	if facetIdx == -1 {
		t.Fatal("$facet stage not found in pipeline")
	}

	facetStage := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
	itemsPipeline := facetStage["items"].(bson.A)

	// Items pipeline: $limit → $lookup (decoration)
	if len(itemsPipeline) < 2 {
		t.Fatalf("items pipeline should have at least 2 stages, got %d", len(itemsPipeline))
	}

	// First stage must be $limit
	if _, ok := itemsPipeline[0].(bson.M)["$limit"]; !ok {
		t.Errorf("first stage in items pipeline should be $limit, got %v", itemsPipeline[0])
	}

	// Second stage must be $lookup (our decoration)
	assertBsonDStageKey(t, itemsPipeline[1], "$lookup", "second stage in items pipeline should be $lookup")

	// $lookup should NOT appear in main pipeline (only inside $facet)
	mainLookupIdx := findStageIndex(pipeline, "$lookup")
	if mainLookupIdx != -1 {
		t.Error("$lookup should not appear in main pipeline when using decoration stages")
	}
}

func TestBuildCursorPipeline_WithDecorationStagesAndProjection(t *testing.T) {
	lookup := bson.D{{"$lookup", bson.M{
		"from": "translations",
		"as":   "_translations",
	}}}

	opts := defaultListCursorOptions()
	WithListCursorDecorationStages(lookup)(opts)
	WithListCursorProjection(bson.M{"name": 1, "code": 1})(opts)

	pipeline := buildCursorPipeline(opts)

	facetStage := pipeline[findStageIndex(pipeline, "$facet")].(bson.M)["$facet"].(bson.M)
	itemsPipeline := facetStage["items"].(bson.A)

	// Items pipeline: $limit → $lookup → $project
	if len(itemsPipeline) != 3 {
		t.Fatalf("items pipeline should have 3 stages ($limit, $lookup, $project), got %d", len(itemsPipeline))
	}

	if _, ok := itemsPipeline[0].(bson.M)["$limit"]; !ok {
		t.Error("first stage should be $limit")
	}
	assertBsonDStageKey(t, itemsPipeline[1], "$lookup", "second stage should be $lookup")
	if _, ok := itemsPipeline[2].(bson.M)["$project"]; !ok {
		t.Error("third stage should be $project")
	}
}

func TestBuildCursorPipeline_WithBothStageTypes(t *testing.T) {
	// Pre-facet stage (for sorting by joined data)
	preFacetLookup := bson.D{{"$lookup", bson.M{
		"from": "categories",
		"as":   "_category",
	}}}
	// Decoration stage (for display-only translation)
	decorationLookup := bson.D{{"$lookup", bson.M{
		"from": "translations",
		"as":   "_translations",
	}}}

	opts := defaultListCursorOptions()
	WithListCursorStages(preFacetLookup)(opts)
	WithListCursorDecorationStages(decorationLookup)(opts)

	pipeline := buildCursorPipeline(opts)

	// Pre-facet $lookup should be in main pipeline between $sort and $facet
	sortIdx := findStageIndex(pipeline, "$sort")
	facetIdx := findStageIndex(pipeline, "$facet")
	mainLookupIdx := findStageIndex(pipeline, "$lookup")

	if mainLookupIdx == -1 {
		t.Fatal("pre-facet $lookup not found in main pipeline")
	}
	if mainLookupIdx <= sortIdx || mainLookupIdx >= facetIdx {
		t.Errorf("pre-facet $lookup (idx=%d) should be between $sort (idx=%d) and $facet (idx=%d)",
			mainLookupIdx, sortIdx, facetIdx)
	}

	// Decoration $lookup should be inside $facet items
	facetStage := pipeline[facetIdx].(bson.M)["$facet"].(bson.M)
	itemsPipeline := facetStage["items"].(bson.A)

	found := false
	for _, stage := range itemsPipeline {
		if d, ok := stage.(bson.D); ok && len(d) > 0 && d[0].Key == "$lookup" {
			if m, ok := d[0].Value.(bson.M); ok && m["from"] == "translations" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("decoration $lookup (translations) not found inside $facet items pipeline")
	}
}

func TestBuildCursorPipeline_EmptyStages(t *testing.T) {
	optsWithStages := defaultListCursorOptions()
	WithListCursorStages()(optsWithStages)
	WithListCursorDecorationStages()(optsWithStages)

	optsDefault := defaultListCursorOptions()

	pipelineWithStages := buildCursorPipeline(optsWithStages)
	pipelineDefault := buildCursorPipeline(optsDefault)

	if len(pipelineWithStages) != len(pipelineDefault) {
		t.Errorf("empty stages should not change pipeline length: with=%d, default=%d",
			len(pipelineWithStages), len(pipelineDefault))
	}
}

func TestBuildFacetStage_WithDecorationStages(t *testing.T) {
	lookup := bson.D{{"$lookup", bson.M{"from": "translations"}}}
	addFields := bson.D{{"$addFields", bson.M{"name": "$_t.name"}}}

	decorationStages := bson.A{lookup, addFields}

	result := buildFacetStage(20, nil, decorationStages, false)

	facet := result[0].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)

	// $limit + 2 decoration stages = 3 total
	if len(items) != 3 {
		t.Fatalf("expected 3 stages in items pipeline, got %d", len(items))
	}

	if _, ok := items[0].(bson.M)["$limit"]; !ok {
		t.Error("first stage should be $limit")
	}
	assertBsonDStageKey(t, items[1], "$lookup", "second stage should be $lookup")
	assertBsonDStageKey(t, items[2], "$addFields", "third stage should be $addFields")
}

func TestBuildFacetStage_NoDecorationStages(t *testing.T) {
	result := buildFacetStage(20, nil, nil, false)

	facet := result[0].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)

	if len(items) != 1 {
		t.Fatalf("expected 1 stage in items pipeline (just $limit), got %d", len(items))
	}
}

func TestBuildFacetStage_LimitIncludesLookahead(t *testing.T) {
	result := buildFacetStage(20, nil, nil, false)

	facet := result[0].(bson.M)["$facet"].(bson.M)
	items := facet["items"].(bson.A)
	limit := items[0].(bson.M)["$limit"]

	if fmt.Sprint(limit) != "21" {
		t.Errorf("$limit should be 21 (20+1 lookahead), got %v", limit)
	}
}

func TestWithListCursorStages_AppendsAcrossCalls(t *testing.T) {
	opts := defaultListCursorOptions()

	stage1 := bson.D{{"$lookup", bson.M{"from": "a"}}}
	stage2 := bson.D{{"$addFields", bson.M{"x": 1}}}

	WithListCursorStages(stage1)(opts)
	WithListCursorStages(stage2)(opts)

	if len(opts.stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(opts.stages))
	}
}

func TestWithListCursorDecorationStages_AppendsAcrossCalls(t *testing.T) {
	opts := defaultListCursorOptions()

	stage1 := bson.D{{"$lookup", bson.M{"from": "a"}}}
	stage2 := bson.D{{"$addFields", bson.M{"x": 1}}}

	WithListCursorDecorationStages(stage1)(opts)
	WithListCursorDecorationStages(stage2)(opts)

	if len(opts.decorationStages) != 2 {
		t.Fatalf("expected 2 decoration stages, got %d", len(opts.decorationStages))
	}
}

// assertBsonDStageKey checks that a pipeline stage is a bson.D with the expected key.
func assertBsonDStageKey(t *testing.T, stage any, expectedKey, msg string) {
	t.Helper()
	d, ok := stage.(bson.D)
	if !ok || len(d) == 0 || d[0].Key != expectedKey {
		t.Errorf("%s: got %v", msg, stage)
	}
}

// findStageIndex returns the index of the first pipeline stage with the given key, or -1.
func findStageIndex(pipeline bson.A, stageKey string) int {
	for i, stage := range pipeline {
		switch s := stage.(type) {
		case bson.M:
			if _, ok := s[stageKey]; ok {
				return i
			}
		case bson.D:
			if len(s) > 0 && s[0].Key == stageKey {
				return i
			}
		}
	}
	return -1
}

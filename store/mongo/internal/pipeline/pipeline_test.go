// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pipeline_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/store/mongo/internal/pipeline"
)

func TestBuildItemsAggregationStages(t *testing.T) {
	t.Parallel()
	t.Run("limit only", func(t *testing.T) {
		t.Parallel()
		got := pipeline.CursorItems(10, nil, nil)
		require.Len(t, got, 1)
		assertBsonDStageKey(t, got[0], "$limit")
		require.Equal(t, int64(11), got[0].(bson.M)["$limit"], "limit should be 10 + lookahead")
	})

	t.Run("limit + decoration", func(t *testing.T) {
		t.Parallel()
		decoration := bson.A{
			bson.D{{Key: "$lookup", Value: bson.M{"from": "translations", "localField": "_id", "foreignField": "entity_id", "as": "translations"}}},
		}
		got := pipeline.CursorItems(10, nil, decoration)
		require.Len(t, got, 2)
		assertBsonDStageKey(t, got[0], "$limit")
		assertBsonDStageKey(t, got[1], "$lookup")
	})

	t.Run("limit + decoration + projection", func(t *testing.T) {
		t.Parallel()
		decoration := bson.A{bson.D{{Key: "$lookup", Value: bson.M{"from": "translations"}}}}
		proj := bson.M{"name": 1}
		got := pipeline.CursorItems(10, proj, decoration)
		require.Len(t, got, 3)
		assertBsonDStageKey(t, got[0], "$limit")
		assertBsonDStageKey(t, got[1], "$lookup")
		assertBsonDStageKey(t, got[2], "$project")
	})

	t.Run("limit + projection (no decoration)", func(t *testing.T) {
		t.Parallel()
		got := pipeline.CursorItems(10, bson.M{"name": 1}, nil)
		require.Len(t, got, 2)
		assertBsonDStageKey(t, got[0], "$limit")
		assertBsonDStageKey(t, got[1], "$project")
	})
}

func TestBuildCountUnionStages(t *testing.T) {
	t.Parallel()
	t.Run("includeTotal=false: only $group + $project total=-1", func(t *testing.T) {
		t.Parallel()
		got := pipeline.CursorResult("test_collection", rawFilter(t, bson.M{"status": "active"}), false)
		require.Len(t, got, 2)
		assertBsonDStageKey(t, got[0], "$group")
		assertBsonDStageKey(t, got[1], "$project")

		// Final $project must set total to -1 (sentinel for "not computed").
		proj := got[1].(bson.M)["$project"].(bson.M)
		require.EqualValues(t, -1, proj["total"], "total should be -1 when includeTotal is false")
	})

	t.Run("includeTotal=true: $replaceRoot + $unionWith + $group + $project", func(t *testing.T) {
		t.Parallel()
		got := pipeline.CursorResult("test_collection", rawFilter(t, bson.M{"status": "active"}), true)
		require.Len(t, got, 4)
		assertBsonDStageKey(t, got[0], "$replaceRoot")
		assertBsonDStageKey(t, got[1], "$unionWith")
		assertBsonDStageKey(t, got[2], "$group")
		assertBsonDStageKey(t, got[3], "$project")
	})

	t.Run("$unionWith targets the same collection", func(t *testing.T) {
		t.Parallel()
		got := pipeline.CursorResult("test_collection", rawFilter(t, bson.M{"status": "active"}), true)
		union := got[1].(bson.M)["$unionWith"].(bson.M)
		require.Equal(t, "test_collection", union["coll"], "$unionWith should target the originating collection")
	})

	t.Run("count sub-pipeline contains user filter but never cursor filter", func(t *testing.T) {
		t.Parallel()
		userFilter := rawFilter(t, bson.M{"status": "active"})
		got := pipeline.CursorResult("test_collection", userFilter, true)
		subPipeline := got[1].(bson.M)["$unionWith"].(bson.M)["pipeline"].(bson.A)

		// Expected: $match (userFilter), $count, $replaceRoot (tag as "count").
		require.Len(t, subPipeline, 3)
		matchStage := subPipeline[0].(bson.M)["$match"].(bson.Raw)
		require.Equal(t, "active", matchStage.Lookup("status").StringValue(), "$match should carry the user filter")
		require.Zero(t, matchStage.Lookup("$and").Type, "user filter should not be wrapped in $and (no cursor_filter expected)")
		require.Zero(t, matchStage.Lookup("cursor_id").Type, "cursor filter must not leak into count sub-pipeline")
		assertBsonDStageKey(t, subPipeline[1], "$count")
		assertBsonDStageKey(t, subPipeline[2], "$replaceRoot")
	})

	t.Run("empty user filter omits $match in count sub-pipeline", func(t *testing.T) {
		t.Parallel()
		got := pipeline.CursorResult("test_collection", nil, true)
		subPipeline := got[1].(bson.M)["$unionWith"].(bson.M)["pipeline"].(bson.A)

		// Expected: $count, $replaceRoot — no $match when filter is empty.
		require.Len(t, subPipeline, 2)
		assertBsonDStageKey(t, subPipeline[0], "$count")
		assertBsonDStageKey(t, subPipeline[1], "$replaceRoot")
	})
}

func assertBsonDStageKey(t testing.TB, stage any, key string) {
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
		require.Failf(t, "missing stage key", "expected %q, got %v", key, s)
	default:
		require.Failf(t, "unexpected stage type", "%T", stage)
	}
}

func TestBuildItemsAggregationStagesSkipsEmptyProjection(t *testing.T) {
	t.Parallel()

	stages := pipeline.CursorItems(10, bson.M{}, nil)
	require.Len(t, stages, 1, "an empty $project is invalid in MongoDB and must be omitted")
}

func TestOffset(t *testing.T) {
	t.Parallel()
	sort := bson.D{{Key: "created_at", Value: -1}}
	for _, tc := range []struct {
		name       string
		filter     bson.Raw
		projection bson.M
		offset     int64
		total      bool
	}{
		{name: "minimal"},
		{name: "with total", total: true},
		{name: "filtered projected page", filter: rawFilter(t, bson.M{"status": "active"}), projection: bson.M{"name": 1}, offset: 40, total: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pipeline.Offset(tc.filter, sort, tc.projection, 20, tc.offset, tc.total)
			index := 0
			if len(tc.filter) > 0 {
				require.Equal(t, bson.M{"$match": tc.filter}, got[index])
				index++
			}
			require.Equal(t, bson.M{"$sort": sort}, got[index])
			index++
			facet := got[index].(bson.M)["$facet"].(bson.M)
			items := bson.A{}
			if tc.offset > 0 {
				items = append(items, bson.M{"$skip": tc.offset})
			}
			items = append(items, bson.M{"$limit": int64(20)})
			if len(tc.projection) > 0 {
				items = append(items, bson.M{"$project": tc.projection})
			}
			require.Equal(t, items, facet["items"])
			if tc.total {
				require.Equal(t, bson.A{bson.M{"$count": "total"}}, facet["count"])
				require.Equal(t, bson.M{"$unwind": bson.M{"path": "$count", "preserveNullAndEmptyArrays": true}}, got[index+1])
				require.Equal(t, bson.M{"$project": bson.M{"items": "$items", "total": bson.M{"$ifNull": bson.A{"$count.total", 0}}}}, got[index+2])
				require.Len(t, got, index+3)
			} else {
				require.NotContains(t, facet, "count")
				require.Equal(t, bson.M{"$project": bson.M{"items": "$items", "total": -1}}, got[index+1])
				require.Len(t, got, index+2)
			}
		})
	}
}

func rawFilter(t testing.TB, doc bson.M) bson.Raw {
	t.Helper()
	raw, err := bson.Marshal(doc)
	require.NoError(t, err)
	return raw
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestProjectionKeeps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		projection bson.M
		path       string
		want       bool
	}{
		{name: "inclusion lists path", projection: bson.M{"a": 1}, path: "a", want: true},
		{name: "inclusion lists ancestor", projection: bson.M{"a": int32(1)}, path: "a.b", want: true},
		{name: "inclusion omits path", projection: bson.M{"a": 1}, path: "b"},
		{name: "inclusion lists only a descendant", projection: bson.M{"a.b": 1}, path: "a"},
		{name: "inclusion keeps _id implicitly", projection: bson.M{"a": 1}, path: "_id", want: true},
		{name: "inclusion suppresses _id", projection: bson.M{"a": 1, "_id": int32(0)}, path: "_id"},
		{name: "exclusion omits path", projection: bson.M{"a": 0}, path: "b", want: true},
		{name: "exclusion of path", projection: bson.M{"a": false}, path: "a"},
		{name: "exclusion of ancestor", projection: bson.M{"a": int64(0)}, path: "a.b"},
		{name: "exclusion of descendant", projection: bson.M{"a.b": 0.0}, path: "a"},
		{name: "only _id excluded is exclusion mode", projection: bson.M{"_id": 0}, path: "a", want: true},
		{name: "computed value is not the stored one", projection: bson.M{"a": "$b"}, path: "a"},
		{name: "literal replacement", projection: bson.M{"_id": 1, "a": bson.M{"$literal": 5}}, path: "a"},
		{name: "conditional removal", projection: bson.M{"a": "$$REMOVE", "b": 1}, path: "a"},
		{name: "computed ancestor", projection: bson.M{"a": bson.M{"$literal": 1}}, path: "a.b"},
		{name: "_id-only inclusion drops others", projection: bson.M{"_id": 1}, path: "a"},
		{name: "_id-only inclusion keeps _id", projection: bson.M{"_id": true}, path: "_id", want: true},
		{name: "_id kept beside an exclusion", projection: bson.M{"_id": 1, "password": 0}, path: "a", want: true},
		{name: "computed field makes inclusion", projection: bson.M{"x": "$y"}, path: "a"},
		{name: "boundary is a dot", projection: bson.M{"ab": 1}, path: "a"},
		{name: "nearest ancestor decides", projection: bson.M{"a": 1, "a.b": 0}, path: "a.b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, projectionKeeps(tc.projection, tc.path))
		})
	}
}

func TestProjectionFlagValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		v                  any
		preserve, excludes bool
	}{
		{name: "true", v: true, preserve: true},
		{name: "false", v: false, excludes: true},
		{name: "int one", v: 1, preserve: true},
		{name: "int zero", v: 0, excludes: true},
		{name: "int32 one", v: int32(1), preserve: true},
		{name: "int32 zero", v: int32(0), excludes: true},
		{name: "int64 one", v: int64(1), preserve: true},
		{name: "int64 zero", v: int64(0), excludes: true},
		{name: "float one", v: 1.0, preserve: true},
		{name: "float zero", v: 0.0, excludes: true},
		{name: "expression", v: "$x"},
		{name: "document", v: bson.M{"$literal": 1}},
		{name: "nil", v: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.preserve, isPreservationFlag(tc.v))
			require.Equal(t, tc.excludes, isExclusionValue(tc.v))
		})
	}
}

func TestValidateCursorProjection(t *testing.T) {
	t.Parallel()

	sortByCreated := bson.D{{Key: "created_at", Value: -1}}
	tests := []struct {
		name       string
		projection bson.M
		sort       bson.D
		wantErr    bool
	}{
		{name: "nil projection", sort: sortByCreated},
		{name: "empty projection", projection: bson.M{}, sort: sortByCreated},
		{name: "keeps both", projection: bson.M{"name": 1, "created_at": 1}, sort: sortByCreated},
		{name: "drops sort field", projection: bson.M{"name": 1}, sort: sortByCreated, wantErr: true},
		{name: "drops cursor id", projection: bson.M{"created_at": 1, "_id": 0}, sort: sortByCreated, wantErr: true},
		{name: "sort on cursor id only", projection: bson.M{"name": 1}, sort: bson.D{{Key: "_id", Value: 1}}},
		{name: "exclusion of sort field", projection: bson.M{"created_at": 0}, sort: sortByCreated, wantErr: true},
		{name: "default sort", projection: bson.M{"name": 1}},
		{name: "_id-only inclusion drops sort field", projection: bson.M{"_id": 1}, sort: sortByCreated, wantErr: true},
		{name: "literal sort field", projection: bson.M{"_id": 1, "created_at": bson.M{"$literal": 0}}, sort: sortByCreated, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateCursorProjection(tc.projection, "_id", tc.sort)
			if tc.wantErr {
				require.ErrorIs(t, err, ErrProjectionDropsCursorField)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestListCursor_RejectsProjectionDroppingSortField pins that the check runs
// before any query: the client never connects to a server here.
func TestListCursor_RejectsProjectionDroppingSortField(t *testing.T) {
	t.Parallel()

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(t.Context()) })
	coll := client.Database("testdb").Collection("items")

	_, err = ListCursor[bson.M](t.Context(), coll,
		WithListCursorSort(bson.D{{Key: "created_at", Value: -1}}),
		WithListCursorProjection(bson.M{"name": 1}),
	)
	require.ErrorIs(t, err, ErrProjectionDropsCursorField)
}

func TestBuildItemsAggregationStagesSkipsEmptyProjection(t *testing.T) {
	t.Parallel()

	stages := buildItemsAggregationStages(10, bson.M{}, nil)
	require.Len(t, stages, 1, "an empty $project is invalid in MongoDB and must be omitted")
}

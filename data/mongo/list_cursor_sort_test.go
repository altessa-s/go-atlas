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

func TestValidateCursorSort(t *testing.T) {
	t.Parallel()

	const id = "_id"
	tests := []struct {
		name    string
		sort    bson.D
		wantErr bool
	}{
		{"empty", nil, false},
		{"single field", bson.D{{Key: "score", Value: 1}}, false},
		{"cursor id only", bson.D{{Key: id, Value: -1}}, false},
		{"field then id same direction", bson.D{{Key: "score", Value: -1}, {Key: id, Value: int32(-1)}}, false},
		{"id first then others", bson.D{{Key: id, Value: 1}, {Key: "score", Value: -1}}, false},
		{"field then id opposite direction", bson.D{{Key: "score", Value: 1}, {Key: id, Value: -1}}, true},
		{"two non-id fields", bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}}, true},
		{"three fields ending in id", bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}, {Key: id, Value: 1}}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateCursorSort(tc.sort, id)
			if tc.wantErr {
				require.ErrorIs(t, err, ErrUnsupportedCursorSort)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestListCursor_RejectsUnsupportedSort pins that the shape check runs before
// any query: the client never connects to a server here.
func TestListCursor_RejectsUnsupportedSort(t *testing.T) {
	t.Parallel()

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(t.Context()) })
	coll := client.Database("testdb").Collection("items")

	_, err = ListCursor[bson.M](t.Context(), coll,
		WithListCursorIdField("_id"),
		WithListCursorSort(bson.D{{Key: "score", Value: 1}, {Key: "_id", Value: -1}}),
	)
	require.ErrorIs(t, err, ErrUnsupportedCursorSort)
}

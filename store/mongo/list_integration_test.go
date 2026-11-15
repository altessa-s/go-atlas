// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/mongotest"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
)

type listItem struct {
	Name  string `bson:"name"`
	Rank  int    `bson:"rank"`
	Group string `bson:"group"`
}

// listColl returns a collection seeded with five items (skipped without MONGO_URI).
func listColl(tb testing.TB) *mongo.Collection {
	tb.Helper()
	m := mongotest.New(tb, mongotest.WithDatabasePrefix("list_it_"))
	coll := m.Database().Collection("items")
	docs := []any{
		listItem{Name: "a", Rank: 1, Group: "x"},
		listItem{Name: "b", Rank: 2, Group: "y"},
		listItem{Name: "c", Rank: 3, Group: "x"},
		listItem{Name: "d", Rank: 4, Group: "x"},
		listItem{Name: "e", Rank: 5, Group: "y"},
	}
	_, err := coll.InsertMany(tb.Context(), docs)
	require.NoError(tb, err)
	return coll
}

func names(items []listItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Name)
	}
	return out
}

// TestListIT pins the List pipeline end to end: $match, $sort, $facet with
// $skip/$limit/$project, and the total transform.
func TestListIT(t *testing.T) {
	t.Parallel()
	coll := listColl(t)

	tests := []struct {
		name      string
		opts      []datamongo.ListOption
		wantNames []string
		wantTotal int64
	}{
		{
			name:      "filter sort and page with total",
			opts:      []datamongo.ListOption{datamongo.WithListFilter(bson.M{"group": "x"}), datamongo.WithListSort("-rank"), datamongo.WithListLimit(2, 1)},
			wantNames: []string{"c", "a"},
			wantTotal: 3,
		},
		{
			name:      "total disabled reports minus one",
			opts:      []datamongo.ListOption{datamongo.WithListSort("rank"), datamongo.WithListLimit(2, 0), datamongo.WithListTotal(false)},
			wantNames: []string{"a", "b"},
			wantTotal: -1,
		},
		{
			name:      "no match yields empty items and zero total",
			opts:      []datamongo.ListOption{datamongo.WithListFilter(bson.M{"group": "none"})},
			wantNames: []string{},
			wantTotal: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := datamongo.List[listItem](t.Context(), coll, tc.opts...)
			require.NoError(t, err)
			require.Equal(t, tc.wantNames, names(res.Items))
			require.Equal(t, tc.wantTotal, res.Total)
		})
	}
}

// TestListIT_Projection checks that the projection is applied to the items.
func TestListIT_Projection(t *testing.T) {
	t.Parallel()
	coll := listColl(t)

	res, err := datamongo.List[listItem](t.Context(), coll,
		datamongo.WithListSort("rank"),
		datamongo.WithListLimit(1, 0),
		datamongo.WithListProjection(bson.M{"name": 1}),
	)
	require.NoError(t, err)
	require.Equal(t, []listItem{{Name: "a"}}, res.Items, "projected-out fields stay zero")
	require.Equal(t, int64(5), res.Total)
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cas_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/mongo/cas"
	"github.com/altessa-s/go-atlas/data/mongo/mongotest"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

var (
	errTestNotFound = errors.New("test: not found")
	errTestConflict = errors.New("test: conflict")
)

// seed inserts docs into a fresh collection of a throwaway database (skipped without MONGO_URI).
func seed(tb testing.TB, docs ...bson.D) *mongo.Collection {
	tb.Helper()
	m := mongotest.New(tb, mongotest.WithDatabasePrefix("cas_it_"))
	coll := m.Database().Collection("docs")
	if len(docs) > 0 {
		_, err := coll.InsertMany(tb.Context(), docs)
		require.NoError(tb, err)
	}
	return coll
}

// TestFilterIT: the filter matches only the document at exactly the expected version; a document without a version or at 0 is
// never matched by a positive expectation.
func TestFilterIT(t *testing.T) {
	t.Parallel()
	coll := seed(t,
		bson.D{{Key: "_id", Value: "missing"}},
		bson.D{{Key: "_id", Value: "zero"}, {Key: "version", Value: int64(0)}},
		bson.D{{Key: "_id", Value: "one"}, {Key: "version", Value: int64(1)}},
		bson.D{{Key: "_id", Value: "five"}, {Key: "version", Value: int64(5)}},
	)
	count := func(id string, expected int64) int64 {
		n, err := coll.CountDocuments(t.Context(), cas.Filter("_id", id, "version", expected))
		require.NoError(t, err)
		return n
	}
	require.EqualValues(t, 0, count("missing", 1))
	require.EqualValues(t, 0, count("zero", 1))
	require.EqualValues(t, 1, count("zero", 0))
	require.EqualValues(t, 1, count("one", 1))
	require.EqualValues(t, 0, count("one", 2))
	require.EqualValues(t, 0, count("five", 4))
	require.EqualValues(t, 1, count("five", 5))
	require.EqualValues(t, 0, count("five", 6))
}

// TestIncIT: a CAS update with Inc and a $set writes the data and v+1 (stored as int64) in one write; the same update with the
// now stale expectation matches nothing.
func TestIncIT(t *testing.T) {
	t.Parallel()
	coll := seed(t, bson.D{{Key: "_id", Value: "a"}, {Key: "version", Value: int64(5)}})

	update := cas.Inc("version")
	update["$set"] = bson.M{"touched": true}
	res, err := coll.UpdateOne(t.Context(), cas.Filter("_id", "a", "version", 5), update)
	require.NoError(t, err)
	require.EqualValues(t, 1, res.ModifiedCount)

	var raw bson.Raw
	require.NoError(t, coll.FindOne(t.Context(), bson.M{"_id": "a"}).Decode(&raw))
	got, ok := raw.Lookup("version").Int64OK()
	require.True(t, ok, "version stored as int64")
	require.Equal(t, int64(6), got)
	require.True(t, raw.Lookup("touched").Boolean())

	res, err = coll.UpdateOne(t.Context(), cas.Filter("_id", "a", "version", 5), update)
	require.NoError(t, err)
	require.Zero(t, res.MatchedCount)
}

// TestClassifyMissIT: an absent document is the caller's not-found error, a present one its conflict error; nil sentinels fall
// back to ErrNotFound and ErrConflict, which match the canonical core/errors sentinels.
func TestClassifyMissIT(t *testing.T) {
	t.Parallel()
	coll := seed(t, bson.D{{Key: "_id", Value: "a"}, {Key: "version", Value: int64(3)}})

	res, err := coll.UpdateOne(t.Context(), cas.Filter("_id", "a", "version", 2), bson.M{"$set": bson.M{"x": 1}})
	require.NoError(t, err)
	require.Zero(t, res.MatchedCount)

	require.ErrorIs(t, cas.ClassifyMiss(t.Context(), coll, "_id", "a", errTestNotFound, errTestConflict), errTestConflict)
	require.ErrorIs(t, cas.ClassifyMiss(t.Context(), coll, "_id", "b", errTestNotFound, errTestConflict), errTestNotFound)
	conflict := cas.ClassifyMiss(t.Context(), coll, "_id", "a", nil, nil)
	require.ErrorIs(t, conflict, cas.ErrConflict)
	require.ErrorIs(t, conflict, coreerrs.ErrConflict)
	notFound := cas.ClassifyMiss(t.Context(), coll, "_id", "b", nil, nil)
	require.ErrorIs(t, notFound, cas.ErrNotFound)
	require.ErrorIs(t, notFound, coreerrs.ErrNotFound)
}

// TestClassifyMissInTransactionIT: inside a transaction the check sees the transaction's own uncommitted insert.
func TestClassifyMissInTransactionIT(t *testing.T) {
	t.Parallel()
	m := mongotest.New(t, mongotest.WithDatabasePrefix("cas_it_"))
	mongotest.RequireReplicaSet(t, m)
	coll := m.Database().Collection("docs")
	require.NoError(t, m.Database().CreateCollection(t.Context(), "docs"))

	err := m.WithTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: "tx"}, {Key: "version", Value: int64(9)}}); err != nil {
			return err
		}
		require.ErrorIs(t, cas.ClassifyMiss(ctx, coll, "_id", "tx", errTestNotFound, errTestConflict), errTestConflict)
		// Outside the transaction the insert is not visible yet.
		require.ErrorIs(t, cas.ClassifyMiss(t.Context(), coll, "_id", "tx", errTestNotFound, errTestConflict), errTestNotFound)
		return nil
	})
	require.NoError(t, err)
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/data/mongo/mongotest"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// indexColl returns a collection of a throwaway database (skipped without MONGO_URI).
func indexColl(tb testing.TB) (*datamongo.Mongo, *mongo.Collection) {
	tb.Helper()
	m := mongotest.New(tb, mongotest.WithDatabasePrefix("index_it_"))
	return m, m.Database().Collection("items")
}

// indexSpec returns the specification of index name, failing when it is absent.
func indexSpec(tb testing.TB, coll *mongo.Collection, name string) mongo.IndexSpecification {
	tb.Helper()
	specs, err := coll.Indexes().ListSpecifications(tb.Context())
	require.NoError(tb, err)
	for _, sp := range specs {
		if sp.Name == name {
			return sp
		}
	}
	require.FailNowf(tb, "index not found", "index %s not found", name)
	return mongo.IndexSpecification{}
}

// indexSince returns when the index name was built (or the server started), from $indexStats.
func indexSince(tb testing.TB, coll *mongo.Collection, name string) time.Time {
	tb.Helper()
	cur, err := coll.Aggregate(tb.Context(), mongo.Pipeline{{{Key: "$indexStats", Value: bson.D{}}}})
	require.NoError(tb, err)
	var stats []struct {
		Name     string `bson:"name"`
		Accesses struct {
			Since time.Time `bson:"since"`
		} `bson:"accesses"`
	}
	require.NoError(tb, cur.All(tb.Context(), &stats))
	for _, s := range stats {
		if s.Name == name {
			return s.Accesses.Since
		}
	}
	require.FailNowf(tb, "no index stats", "no stats for index %s", name)
	return time.Time{}
}

func isConflict(err error) bool {
	se, ok := errors.AsType[mongo.ServerError](err)
	return ok && (se.HasErrorCode(datamongo.MongoErrorCodeIndexOptionsConflict) || se.HasErrorCode(datamongo.MongoErrorCodeIndexKeySpecsConflict))
}

// TestEnsureIndexesIT: named and unnamed models are created with the driver's names, and a second run is a no-op.
func TestEnsureIndexesIT(t *testing.T) {
	t.Parallel()
	m, coll := indexColl(t)
	models := []mongo.IndexModel{
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "code", Value: 1}}, Options: mongoOptions.Index().SetName("uniq_code").SetUnique(true)},
		{Keys: bson.D{{Key: "title", Value: "text"}}},
	}
	require.NoError(t, datamongo.EnsureIndexes(t.Context(), coll, models))
	require.Equal(t, []string{"_id_", "status_1_created_at_-1", "title_text", "uniq_code"}, mongotest.IndexNames(t, m, "items"))

	since := indexSince(t, coll, "uniq_code")
	require.NoError(t, datamongo.EnsureIndexes(t.Context(), coll, models, datamongo.WithDropOnSpecMismatch()))
	require.Equal(t, since, indexSince(t, coll, "uniq_code"), "an equal index is not rebuilt")
	require.NoError(t, datamongo.EnsureIndexes(t.Context(), coll, nil))
}

// TestEnsureIndexesConflictIT: a model that conflicts with an existing index of its name fails without WithDropOnSpecMismatch and
// replaces it with the option, for other keys and for other options.
func TestEnsureIndexesConflictIT(t *testing.T) {
	t.Parallel()
	_, coll := indexColl(t)
	stale := []mongo.IndexModel{
		{Keys: bson.D{{Key: "botID", Value: 1}, {Key: "matchedAt", Value: -1}}, Options: mongoOptions.Index().SetName("idx_bot_matched_at")},
		{Keys: bson.D{{Key: "code", Value: 1}}, Options: mongoOptions.Index().SetName("idx_code")},
		{Keys: bson.D{{Key: "kept", Value: 1}}, Options: mongoOptions.Index().SetName("idx_kept")},
	}
	require.NoError(t, datamongo.EnsureIndexes(t.Context(), coll, stale))
	keptSince := indexSince(t, coll, "idx_kept")

	current := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "botID", Value: 1}, {Key: "matchedAt", Value: -1}, {Key: "_id", Value: -1}},
			Options: mongoOptions.Index().SetName("idx_bot_matched_at"),
		},
		{Keys: bson.D{{Key: "code", Value: 1}}, Options: mongoOptions.Index().SetName("idx_code").SetUnique(true)},
		{Keys: bson.D{{Key: "kept", Value: 1}}, Options: mongoOptions.Index().SetName("idx_kept")},
	}
	err := datamongo.EnsureIndexes(t.Context(), coll, current)
	require.Error(t, err)
	require.True(t, isConflict(err), "got %v", err)
	_, hasID := indexSpec(t, coll, "idx_bot_matched_at").KeysDocument.Lookup("_id").Int32OK()
	require.False(t, hasID, "the stale index is untouched without the option")

	require.NoError(t, datamongo.EnsureIndexes(t.Context(), coll, current, datamongo.WithDropOnSpecMismatch()))
	keys := indexSpec(t, coll, "idx_bot_matched_at").KeysDocument
	require.Equal(t, int32(-1), keys.Lookup("_id").Int32())
	unique := indexSpec(t, coll, "idx_code").Unique
	require.NotNil(t, unique)
	require.True(t, *unique)
	require.Equal(t, keptSince, indexSince(t, coll, "idx_kept"), "the matching index is not rebuilt")
}

// TestEnsureIndexesForeignConflictIT: the same keys under another name are never dropped, even with WithDropOnSpecMismatch.
func TestEnsureIndexesForeignConflictIT(t *testing.T) {
	t.Parallel()
	m, coll := indexColl(t)
	_, err := coll.Indexes().CreateOne(t.Context(), mongo.IndexModel{
		Keys: bson.D{{Key: "a", Value: 1}}, Options: mongoOptions.Index().SetName("legacy_a"),
	})
	require.NoError(t, err)

	err = datamongo.EnsureIndexes(t.Context(), coll, []mongo.IndexModel{
		{Keys: bson.D{{Key: "a", Value: 1}}, Options: mongoOptions.Index().SetName("idx_a")},
	}, datamongo.WithDropOnSpecMismatch())
	require.Error(t, err)
	require.True(t, isConflict(err), "got %v", err)
	require.Equal(t, []string{"_id_", "legacy_a"}, mongotest.IndexNames(t, m, "items"))

	// Dropping it explicitly as obsolete lets the model in.
	require.NoError(t, datamongo.EnsureIndexes(t.Context(), coll, []mongo.IndexModel{
		{Keys: bson.D{{Key: "a", Value: 1}}, Options: mongoOptions.Index().SetName("idx_a")},
	}, datamongo.WithDropObsolete("legacy_a")))
	require.Equal(t, []string{"_id_", "idx_a"}, mongotest.IndexNames(t, m, "items"))
}

// TestEnsureIndexesDropObsoleteIT: obsolete indexes are dropped when present; a missing index or collection is fine.
func TestEnsureIndexesDropObsoleteIT(t *testing.T) {
	t.Parallel()
	m, coll := indexColl(t)
	_, err := coll.Indexes().CreateOne(t.Context(), mongo.IndexModel{
		Keys: bson.D{{Key: "effective_date", Value: 1}}, Options: mongoOptions.Index().SetName("idx_effective_date"),
	})
	require.NoError(t, err)

	require.NoError(t, datamongo.EnsureIndexes(t.Context(), coll, []mongo.IndexModel{
		{Keys: bson.D{{Key: "effective_date", Value: -1}, {Key: "_id", Value: 1}}, Options: mongoOptions.Index().SetName("idx_effective_id")},
	}, datamongo.WithDropObsolete("idx_effective_date", "never_existed")))
	require.Equal(t, []string{"_id_", "idx_effective_id"}, mongotest.IndexNames(t, m, "items"))

	missing := m.Database().Collection("missing")
	require.NoError(t, datamongo.EnsureIndexes(t.Context(), missing, nil, datamongo.WithDropObsolete("whatever")))
}

// TestEnsureIndexesConcurrentReplacementIT: replicas replacing a non-unique index by a unique one at the same time all succeed, and
// the unique constraint exists at the end: no replica drops the index a peer has just built.
func TestEnsureIndexesConcurrentReplacementIT(t *testing.T) {
	t.Parallel()
	m, coll1 := indexColl(t)
	other, err := datamongo.New(m.DatabaseName(), datamongo.WithClientOptions(mongoOptions.Client().ApplyURI(os.Getenv(mongotest.DefaultEnv))))
	require.NoError(t, err)
	require.NoError(t, other.Connect(t.Context()))
	t.Cleanup(other.Close)
	coll2 := other.Database().Collection("items")

	docs := make([]any, 50)
	for i := range docs {
		docs[i] = bson.D{{Key: "code", Value: i}}
	}
	_, err = coll1.InsertMany(t.Context(), docs)
	require.NoError(t, err)
	desired := []mongo.IndexModel{{Keys: bson.D{{Key: "code", Value: 1}}, Options: mongoOptions.Index().SetName("idx_code").SetUnique(true)}}

	for round := range 3 {
		if dropErr := coll1.Indexes().DropOne(t.Context(), "idx_code"); dropErr != nil {
			require.True(t, datamongo.IsErrorIndexNotFound(dropErr), "round %d: %v", round, dropErr)
		}
		_, err = coll1.Indexes().CreateOne(t.Context(), mongo.IndexModel{
			Keys: bson.D{{Key: "code", Value: 1}}, Options: mongoOptions.Index().SetName("idx_code"),
		})
		require.NoError(t, err)

		err = concurrency.Process(t.Context(), []int{0, 1, 2, 3, 4, 5, 6, 7}, func(ctx context.Context, i int) error {
			coll := coll1
			if i%2 == 1 {
				coll = coll2
			}
			return datamongo.EnsureIndexes(ctx, coll, desired, datamongo.WithDropOnSpecMismatch())
		}, concurrency.WithConcurrency[int](8))
		require.NoError(t, err, "round %d", round)

		unique := indexSpec(t, coll1, "idx_code").Unique
		require.NotNil(t, unique, "round %d", round)
		require.True(t, *unique, "round %d", round)
		_, err = coll1.InsertOne(t.Context(), bson.D{{Key: "code", Value: 7}})
		dup, _ := datamongo.IsErrorDuplicate(err)
		require.True(t, dup, "round %d: a duplicate must be rejected, got %v", round, err)
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auditit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/storagetest"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	auditmongo "github.com/altessa-s/go-atlas/audit/storages/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func openMongoStorage(t *testing.T) (*auditmongo.Storage, *mongo.Database) {
	t.Helper()
	db := livebackend.MongoDatabase(t, "audit")
	s, err := auditmongo.New(t.Context(), db)
	require.NoError(t, err)
	return s, db
}

func newMongoStorage(t *testing.T) audit.Storage {
	t.Helper()
	s, _ := openMongoStorage(t)
	return s
}

func TestMongoConformance(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, newMongoStorage, nil)
}

func TestMongoStoreBatchReplay(t *testing.T) {
	t.Parallel()
	s := newMongoStorage(t)
	at := time.Now().UTC().Truncate(time.Millisecond)
	e1 := event("e1", at)
	e2 := event("e2", at.Add(time.Second))
	e3 := event("e3", at.Add(2*time.Second))

	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{e1, e2}))
	require.NoError(t, s.StoreBatch(t.Context(), []*audit.Event{e1, e2, e3}))
	require.Equal(t, []string{"e1", "e2", "e3"}, ids(t, s))

	count, err := s.Count(t.Context(), nil)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)
}

func TestMongoStoreBatchPartialFailure(t *testing.T) {
	t.Parallel()

	t.Run("secondary_unique_index", func(t *testing.T) {
		t.Parallel()
		s, db := openMongoStorage(t)
		coll := db.Collection(auditmongo.DefaultCollectionName)
		_, err := coll.Indexes().CreateOne(t.Context(), mongo.IndexModel{
			Keys:    bson.D{{Key: "actor.email", Value: 1}},
			Options: mongoOptions.Index().SetUnique(true).SetSparse(true),
		})
		require.NoError(t, err)

		at := time.Now().UTC().Truncate(time.Millisecond)
		e1 := event("e1", at)
		e1.Actor.Email = "unique-marker@example.com"
		require.NoError(t, s.Store(t.Context(), e1))

		conflict := event("e2", at.Add(time.Second))
		conflict.Actor.Email = "unique-marker@example.com"
		e3 := event("e3", at.Add(2*time.Second))

		writeErr := storagetest.BatchFailure(t, s, []*audit.Event{e1, conflict, e3}, []string{"e1", "e3"})
		require.NotContains(t, writeErr.Error(), "unique-marker@example.com")
	})

	t.Run("validator", func(t *testing.T) {
		t.Parallel()
		s, db := openMongoStorage(t)
		at := time.Now().UTC().Truncate(time.Millisecond)
		e1 := event("e1", at)
		require.NoError(t, s.Store(t.Context(), e1))

		require.NoError(t, db.RunCommand(t.Context(), bson.D{
			{Key: "collMod", Value: auditmongo.DefaultCollectionName},
			{Key: "validator", Value: bson.D{
				{Key: "actor.id", Value: bson.D{{Key: "$ne", Value: "boom-marker"}}},
			}},
		}).Err())

		rejected := event("e2", at.Add(time.Second))
		rejected.Actor.ID = "boom-marker"
		e3 := event("e3", at.Add(2*time.Second))

		writeErr := storagetest.BatchFailure(t, s, []*audit.Event{e1, rejected, e3}, []string{"e1", "e3"})
		require.NotContains(t, writeErr.Error(), "boom-marker")
	})
}

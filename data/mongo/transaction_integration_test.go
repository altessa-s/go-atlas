// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/data/mongo/mongotest"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestInTransactionWithoutSession: a plain context carries no transaction.
func TestInTransactionWithoutSession(t *testing.T) {
	t.Parallel()
	require.False(t, datamongo.InTransaction(t.Context()))
}

// txMongo connects to a replica set (skipped without MONGO_URI) and creates the collection docs, which a transaction cannot create
// on servers before 4.4.
func txMongo(t *testing.T) (*datamongo.Mongo, *mongo.Collection) {
	t.Helper()
	m := mongotest.New(t, mongotest.WithDatabasePrefix("tx_it_"))
	mongotest.RequireReplicaSet(t, m)
	require.NoError(t, m.Database().CreateCollection(t.Context(), "docs"))
	return m, m.Database().Collection("docs")
}

// TestInTransactionIT: true inside a transaction, false for a session without one and after the commit.
func TestInTransactionIT(t *testing.T) {
	t.Parallel()
	m, _ := txMongo(t)

	sess, err := m.Client().StartSession()
	require.NoError(t, err)
	defer sess.EndSession(context.WithoutCancel(t.Context()))
	sessCtx := mongo.NewSessionContext(t.Context(), sess)
	require.False(t, datamongo.InTransaction(sessCtx), "a session without a transaction")

	var inside context.Context
	require.NoError(t, m.WithTransaction(t.Context(), func(ctx context.Context) error {
		inside = ctx
		require.True(t, datamongo.InTransaction(ctx))
		return nil
	}))
	require.False(t, datamongo.InTransaction(inside), "committed")
}

// TestWithTransactionJoinsIT: a nested WithTransaction of the same client runs in the enclosing transaction — same session, its
// writes abort with the enclosing transaction, its error reaches the enclosing handler.
func TestWithTransactionJoinsIT(t *testing.T) {
	t.Parallel()
	m, coll := txMongo(t)
	errAbort := errors.New("abort outer")

	err := m.WithTransaction(t.Context(), func(outer context.Context) error {
		outerSess := mongo.SessionFromContext(outer)
		require.NoError(t, m.WithTransaction(outer, func(inner context.Context) error {
			require.Same(t, outerSess, mongo.SessionFromContext(inner), "joined, not a new session")
			_, err := coll.InsertOne(inner, bson.D{{Key: "_id", Value: "inner"}})
			return err
		}))
		n, err := coll.CountDocuments(outer, bson.M{"_id": "inner"})
		require.NoError(t, err)
		require.EqualValues(t, 1, n, "the enclosing transaction sees the joined write")
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	n, err := coll.CountDocuments(t.Context(), bson.M{"_id": "inner"})
	require.NoError(t, err)
	require.Zero(t, n, "the joined write aborted with the enclosing transaction")

	errInner := errors.New("inner failed")
	err = m.WithTransaction(t.Context(), func(outer context.Context) error {
		return m.WithTransaction(outer, func(context.Context) error { return errInner })
	})
	require.ErrorIs(t, err, errInner)
}

// TestWithTransactionDoesNotJoinOtherClientIT: a transaction of another client is never joined; the nested call commits on its own.
func TestWithTransactionDoesNotJoinOtherClientIT(t *testing.T) {
	t.Parallel()
	m1, _ := txMongo(t)
	m2, coll2 := txMongo(t)

	err := m1.WithTransaction(t.Context(), func(outer context.Context) error {
		require.NoError(t, m2.WithTransaction(outer, func(inner context.Context) error {
			require.NotSame(t, mongo.SessionFromContext(outer), mongo.SessionFromContext(inner))
			_, err := coll2.InsertOne(inner, bson.D{{Key: "_id", Value: "independent"}})
			return err
		}))
		return errors.New("abort m1")
	})
	require.Error(t, err)
	n, err := coll2.CountDocuments(t.Context(), bson.M{"_id": "independent"})
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "the other client's transaction committed independently")
}

// TestUpsertRetryingDuplicateInTransactionIT: inside a transaction a duplicate is not retried.
func TestUpsertRetryingDuplicateInTransactionIT(t *testing.T) {
	t.Parallel()
	m, _ := txMongo(t)

	calls := 0
	require.NoError(t, m.WithTransaction(t.Context(), func(ctx context.Context) error {
		calls = 0 // the driver may rerun the callback
		err := datamongo.UpsertRetryingDuplicate(ctx, 3, func(context.Context) error {
			calls++
			return errDuplicate
		})
		require.True(t, mongo.IsDuplicateKeyError(err))
		return nil
	}))
	require.Equal(t, 1, calls)
}

// TestUpsertRetryingDuplicateRaceIT: concurrent first upserts of one document all succeed and leave one document.
func TestUpsertRetryingDuplicateRaceIT(t *testing.T) {
	t.Parallel()
	m := mongotest.New(t, mongotest.WithDatabasePrefix("tx_it_"))
	coll := m.Database().Collection("buckets")

	const writers = 16
	err := concurrency.Process(t.Context(), make([]int, writers), func(ctx context.Context, _ int) error {
		return datamongo.UpsertRetryingDuplicate(ctx, 3, func(ctx context.Context) error {
			_, err := coll.UpdateOne(ctx, bson.M{"_id": "2026-10-18"}, bson.M{"$inc": bson.M{"calls": 1}},
				mongoOptions.UpdateOne().SetUpsert(true))
			return err
		})
	}, concurrency.WithConcurrency[int](writers))
	require.NoError(t, err)
	var doc struct {
		Calls int `bson:"calls"`
	}
	require.NoError(t, coll.FindOne(t.Context(), bson.M{"_id": "2026-10-18"}).Decode(&doc))
	require.Equal(t, writers, doc.Calls)
}

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

	"github.com/altessa-s/go-atlas/data/mongo/mongotest"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
)

// TestTouchGuardEmptyField: an empty guard field is rejected before any write.
func TestTouchGuardEmptyField(t *testing.T) {
	t.Parallel()

	touched, err := datamongo.TouchGuard(t.Context(), nil, bson.M{"_id": "a"}, "", false)
	require.ErrorIs(t, err, datamongo.ErrGuardFieldRequired)
	require.False(t, touched)
}

// guardValue returns the stored guard counter of id and whether it is an int32 (false: int64).
func guardValue(t *testing.T, coll *mongo.Collection, id string) (int64, bool) {
	t.Helper()
	var raw bson.Raw
	require.NoError(t, coll.FindOne(t.Context(), bson.M{"_id": id}).Decode(&raw))
	v := raw.Lookup("guard")
	if i, ok := v.Int32OK(); ok {
		return int64(i), true
	}
	i, ok := v.Int64OK()
	require.True(t, ok, "guard of %s is %v", id, v.Type)
	return i, false
}

// TestTouchGuardIT: a matched document is touched (int32 counter, an int64 one stays int64), a miss touches nothing, an upsert
// inserts the guard with the filter's equality fields.
func TestTouchGuardIT(t *testing.T) {
	t.Parallel()
	m := mongotest.New(t, mongotest.WithDatabasePrefix("guard_it_"))
	coll := m.Database().Collection("targets")
	_, err := coll.InsertMany(t.Context(), []any{
		bson.D{{Key: "_id", Value: "live"}},
		bson.D{{Key: "_id", Value: "deleted"}, {Key: "deleted", Value: true}},
		bson.D{{Key: "_id", Value: "wide"}, {Key: "guard", Value: int64(7)}},
	})
	require.NoError(t, err)
	notDeleted := func(id string) bson.M { return bson.M{"_id": id, "deleted": bson.M{"$ne": true}} }

	touched, err := datamongo.TouchGuard(t.Context(), coll, notDeleted("live"), "guard", false)
	require.NoError(t, err)
	require.True(t, touched)
	touched, err = datamongo.TouchGuard(t.Context(), coll, notDeleted("live"), "guard", false)
	require.NoError(t, err)
	require.True(t, touched)
	v, is32 := guardValue(t, coll, "live")
	require.Equal(t, int64(2), v)
	require.True(t, is32)

	touched, err = datamongo.TouchGuard(t.Context(), coll, notDeleted("wide"), "guard", false)
	require.NoError(t, err)
	require.True(t, touched)
	v, is32 = guardValue(t, coll, "wide")
	require.Equal(t, int64(8), v)
	require.False(t, is32, "an int64 counter stays int64")

	touched, err = datamongo.TouchGuard(t.Context(), coll, notDeleted("deleted"), "guard", false)
	require.NoError(t, err)
	require.False(t, touched, "a soft-deleted target does not match")
	touched, err = datamongo.TouchGuard(t.Context(), coll, notDeleted("absent"), "guard", false)
	require.NoError(t, err)
	require.False(t, touched)
	n, err := coll.CountDocuments(t.Context(), bson.M{"_id": "absent"})
	require.NoError(t, err)
	require.Zero(t, n, "without upsert a miss inserts nothing")

	locks := m.Database().Collection("locks")
	touched, err = datamongo.TouchGuard(t.Context(), locks, bson.M{"_id": "src:2026-10-18"}, "n", true)
	require.NoError(t, err)
	require.True(t, touched)
	touched, err = datamongo.TouchGuard(t.Context(), locks, bson.M{"_id": "src:2026-10-18"}, "n", true)
	require.NoError(t, err)
	require.True(t, touched)
	var lock bson.M
	require.NoError(t, locks.FindOne(t.Context(), bson.M{"_id": "src:2026-10-18"}).Decode(&lock))
	require.Equal(t, bson.M{"_id": "src:2026-10-18", "n": int32(2)}, lock)
}

// TestTouchGuardInt64IncrementIT: WithInt64Increment stores int64 on a new upsert and on an existing int32 counter (promoted), and
// an int64 counter stays int64; the default stores int32 on a new upsert.
func TestTouchGuardInt64IncrementIT(t *testing.T) {
	t.Parallel()
	m := mongotest.New(t, mongotest.WithDatabasePrefix("guard_it_"))
	coll := m.Database().Collection("guards")
	_, err := coll.InsertMany(t.Context(), []any{
		bson.D{{Key: "_id", Value: "old64"}, {Key: "guard", Value: int64(5)}},
		bson.D{{Key: "_id", Value: "old32"}, {Key: "guard", Value: int32(5)}},
	})
	require.NoError(t, err)

	for _, id := range []string{"new64", "old64", "old32"} {
		touched, err := datamongo.TouchGuard(t.Context(), coll, bson.M{"_id": id}, "guard", true, datamongo.WithInt64Increment())
		require.NoError(t, err)
		require.True(t, touched)
	}
	touched, err := datamongo.TouchGuard(t.Context(), coll, bson.M{"_id": "new32"}, "guard", true)
	require.NoError(t, err)
	require.True(t, touched)

	for id, want := range map[string]struct {
		v    int64
		is32 bool
	}{"new64": {1, false}, "old64": {6, false}, "old32": {6, false}, "new32": {1, true}} {
		v, is32 := guardValue(t, coll, id)
		require.Equal(t, want.v, v, id)
		require.Equal(t, want.is32, is32, id)
	}
}

// TestTouchGuardSerializesTransactionsIT: while one transaction holds a touched guard, a second transaction that touches it or
// deletes it gets a transient write conflict, which WithTransaction retries.
func TestTouchGuardSerializesTransactionsIT(t *testing.T) {
	t.Parallel()
	m := mongotest.New(t, mongotest.WithDatabasePrefix("guard_it_"))
	mongotest.RequireReplicaSet(t, m)
	coll := m.Database().Collection("targets")
	_, err := coll.InsertOne(t.Context(), bson.D{{Key: "_id", Value: "t"}})
	require.NoError(t, err)

	begin := func() (*mongo.Session, context.Context) {
		sess, err := m.Client().StartSession()
		require.NoError(t, err)
		t.Cleanup(func() { sess.EndSession(context.WithoutCancel(t.Context())) })
		require.NoError(t, sess.StartTransaction())
		return sess, mongo.NewSessionContext(t.Context(), sess)
	}

	holder, holderCtx := begin()
	touched, err := datamongo.TouchGuard(holderCtx, coll, bson.M{"_id": "t"}, "guard", false)
	require.NoError(t, err)
	require.True(t, touched)

	t.Run("second touch conflicts", func(t *testing.T) {
		other, otherCtx := begin()
		_, err := datamongo.TouchGuard(otherCtx, coll, bson.M{"_id": "t"}, "guard", false)
		require.Error(t, err)
		require.True(t, datamongo.IsTransientTransaction(err) || hasTransientLabel(err), "got %v", err)
		_ = other.AbortTransaction(context.WithoutCancel(t.Context()))
	})
	t.Run("delete conflicts", func(t *testing.T) {
		other, otherCtx := begin()
		_, err := coll.DeleteOne(otherCtx, bson.M{"_id": "t"})
		require.Error(t, err)
		require.True(t, datamongo.IsTransientTransaction(err) || hasTransientLabel(err), "got %v", err)
		_ = other.AbortTransaction(context.WithoutCancel(t.Context()))
	})

	require.NoError(t, holder.CommitTransaction(t.Context()))
	v, _ := guardValue(t, coll, "t")
	require.Equal(t, int64(1), v)
}

// hasTransientLabel reports whether err carries the TransientTransactionError label.
func hasTransientLabel(err error) bool {
	se, ok := errors.AsType[mongo.ServerError](err)
	return ok && se.HasErrorLabel("TransientTransactionError")
}

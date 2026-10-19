// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/mongo/mongotest"
	"github.com/altessa-s/go-atlas/data/retryqueue"
	"github.com/altessa-s/go-atlas/data/retryqueue/storages/storagetest"

	retrymongo "github.com/altessa-s/go-atlas/data/retryqueue/storages/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collName = "retry_items"

var t0 = storagetest.T0

// newColl returns the test collection of a throwaway database; the test skips
// when MONGO_URI is unset.
func newColl(tb testing.TB) *mongo.Collection {
	tb.Helper()
	return mongotest.New(tb).Database().Collection(collName)
}

// newStore returns a Store with its indexes over a fresh collection.
func newStore[P any](tb testing.TB, coll *mongo.Collection, opts ...retrymongo.Option) *retrymongo.Store[P] {
	tb.Helper()
	s, err := retrymongo.New[P](coll, opts...)
	require.NoError(tb, err)
	require.NoError(tb, s.EnsureIndexes(tb.Context()))
	return s
}

// rawDoc returns the stored document of key undecoded.
func rawDoc(tb testing.TB, coll *mongo.Collection, key string) bson.Raw {
	tb.Helper()
	raw, err := coll.FindOne(tb.Context(), bson.M{"_id": key}).Raw()
	require.NoError(tb, err)
	return raw
}

// fields returns the sorted top-level field names of raw.
func fields(tb testing.TB, raw bson.Raw) []string {
	tb.Helper()
	elems, err := raw.Elements()
	require.NoError(tb, err)
	out := make([]string, 0, len(elems))
	for _, e := range elems {
		out = append(out, e.Key())
	}
	slices.Sort(out)
	return out
}

// has reports whether raw has a top-level field.
func has(raw bson.Raw, field string) bool {
	_, err := raw.LookupErr(field)
	return err == nil
}

func TestStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storagetest.Storage {
		return newStore[storagetest.Payload](tb, newColl(tb))
	})
}

func TestStorageContractCustomFlagFields(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storagetest.Storage {
		return newStore[storagetest.Payload](tb, newColl(tb),
			retrymongo.WithFlagField("needsManualRate"), retrymongo.WithFlagReasonField("manualRateReason"),
			retrymongo.WithResolvedRetention(7*24*time.Hour))
	})
}

// TestWireFormat pins the document layout: the payload is inlined, optional
// dates and the flag are absent until set, and the flag uses the configured
// field names.
func TestWireFormat(t *testing.T) {
	t.Parallel()

	coll := newColl(t)
	s := newStore[storagetest.Payload](t, coll,
		retrymongo.WithFlagField("needsManualRate"), retrymongo.WithFlagReasonField("manualRateReason"))
	_, err := s.Mark(t.Context(), storagetest.Pending("k", t0))
	require.NoError(t, err)

	raw := rawDoc(t, coll, "k")
	require.Equal(t, []string{"_id", "attempts", "generation", "nextAttemptAt", "note", "since", "state", "valueOn"}, fields(t, raw))
	require.Equal(t, t0, raw.Lookup("since").Time().UTC())
	require.Equal(t, t0, raw.Lookup("nextAttemptAt").Time().UTC())
	require.Equal(t, t0.Add(-time.Hour), raw.Lookup("valueOn").Time().UTC())
	require.Equal(t, "first", raw.Lookup("note").StringValue())
	require.Equal(t, "pending", raw.Lookup("state").StringValue())
	require.Equal(t, int64(1), raw.Lookup("generation").AsInt64())
	require.Equal(t, int64(0), raw.Lookup("attempts").AsInt64())

	flagged, err := s.Flag(t.Context(), "k", 1, "EUR/RUB")
	require.NoError(t, err)
	require.True(t, flagged)
	raw = rawDoc(t, coll, "k")
	require.True(t, raw.Lookup("needsManualRate").Boolean())
	require.Equal(t, "EUR/RUB", raw.Lookup("manualRateReason").StringValue())
	require.False(t, has(raw, retrymongo.DefaultFlagField))

	require.NoError(t, s.Resolve(t.Context(), "k", 1, t0))
	raw = rawDoc(t, coll, "k")
	require.Equal(t, "resolved", raw.Lookup("state").StringValue())
	require.Equal(t, t0, raw.Lookup("resolvedAt").Time().UTC())
	for _, field := range []string{"needsManualRate", "manualRateReason", "expiredAt"} {
		require.False(t, has(raw, field), field)
	}
}

// TestPendingItemHasNoResolvedAt guards the TTL index: a pending item must not
// carry a resolvedAt (a stored zero date would be purged at once) or an
// expiredAt.
func TestPendingItemHasNoResolvedAt(t *testing.T) {
	t.Parallel()

	coll := newColl(t)
	s := newStore[storagetest.Payload](t, coll, retrymongo.WithResolvedRetention(time.Hour))
	_, err := s.Mark(t.Context(), storagetest.Pending("k20", t0))
	require.NoError(t, err)
	_, err = s.Reopen(t.Context(), storagetest.Item{Key: "k21", Generation: 1, Since: t0, NextAttemptAt: t0})
	require.NoError(t, err)
	for _, key := range []string{"k20", "k21"} {
		raw := rawDoc(t, coll, key)
		require.False(t, has(raw, "resolvedAt"), key)
		require.False(t, has(raw, "expiredAt"), key)
	}
}

// TestLegacyDocumentWithoutGeneration asserts a document written before
// generations reads as generation 0, accepts transitions at 0, moves to a
// generation on Reopen, and is deletable as obsolete.
func TestLegacyDocumentWithoutGeneration(t *testing.T) {
	t.Parallel()

	coll := newColl(t)
	s := newStore[storagetest.Payload](t, coll)
	for _, key := range []string{"legacy", "legacy2"} {
		_, err := coll.InsertOne(t.Context(), bson.M{
			"_id": key, "valueOn": t0, "since": t0, "nextAttemptAt": t0, "state": "pending", "attempts": 0,
		})
		require.NoError(t, err)
	}

	got, err := s.ListDue(t.Context(), t0, 0)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Zero(t, got[0].Generation)

	require.NoError(t, s.Reschedule(t.Context(), "legacy", 0, t0.Add(time.Minute)))
	flagged, err := s.Flag(t.Context(), "legacy", 0, "why")
	require.NoError(t, err)
	require.True(t, flagged)

	reopened, err := s.Reopen(t.Context(), storagetest.Item{Key: "legacy", Since: t0, NextAttemptAt: t0, Generation: 1})
	require.NoError(t, err)
	require.Equal(t, int64(1), reopened.Generation)
	require.Zero(t, reopened.Attempts)
	require.False(t, reopened.Flagged)

	newer, err := s.DeleteObsolete(t.Context(), "legacy2", 0)
	require.NoError(t, err)
	require.False(t, newer)
	ok, err := s.Expire(t.Context(), "legacy2", 0, t0, t0)
	require.NoError(t, err)
	require.False(t, ok, "deleted")
}

// TestUnitPayload asserts an item without caller data (an empty struct
// payload) stores only the store's fields.
func TestUnitPayload(t *testing.T) {
	t.Parallel()

	coll := newColl(t)
	s := newStore[struct{}](t, coll)
	got, err := s.Mark(t.Context(), retryqueue.Item[struct{}]{Key: "m1", Since: t0, NextAttemptAt: t0.Add(time.Hour)})
	require.NoError(t, err)
	require.Equal(t, retryqueue.StatePending, got.State)
	require.NoError(t, s.WakeDue(t.Context(), t0))
	due, err := s.ListDue(t.Context(), t0, 0)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, []string{"_id", "attempts", "generation", "nextAttemptAt", "since", "state"}, fields(t, rawDoc(t, coll, "m1")))
}

// TestReopenRewritesDocument asserts Reopen leaves only the fresh item's fields:
// an omitted payload field, the custom-named flag fields and any field of the
// previous generation are gone.
func TestReopenRewritesDocument(t *testing.T) {
	t.Parallel()

	coll := newColl(t)
	s := newStore[storagetest.Payload](t, coll,
		retrymongo.WithFlagField("needsManualRate"), retrymongo.WithFlagReasonField("manualRateReason"))
	first := storagetest.Pending("k", t0)
	first.Payload.Tag = "old"
	_, err := s.Mark(t.Context(), first)
	require.NoError(t, err)
	_, err = s.Flag(t.Context(), "k", 1, "EUR/RUB")
	require.NoError(t, err)
	_, err = coll.UpdateOne(t.Context(), bson.M{"_id": "k"}, bson.M{"$set": bson.M{"stale": true}})
	require.NoError(t, err)

	_, err = s.Reopen(t.Context(), storagetest.Item{Key: "k", Generation: 2, Since: t0, NextAttemptAt: t0,
		Payload: storagetest.Payload{ValueOn: t0, Note: "second"}})
	require.NoError(t, err)
	require.Equal(t, []string{"_id", "attempts", "generation", "nextAttemptAt", "note", "since", "state", "valueOn"},
		fields(t, rawDoc(t, coll, "k")))
}

// TestIndexNames asserts the exact index set with and without retention: every
// query is backed (see the README audit) and no index is left behind.
func TestIndexNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []retrymongo.Option
		want []string
	}{
		{"no retention", nil, []string{"_id_", retrymongo.IndexPendingNextAttempt, retrymongo.IndexStateSince}},
		{"sub-second retention", []retrymongo.Option{retrymongo.WithResolvedRetention(time.Millisecond)},
			[]string{"_id_", retrymongo.IndexPendingNextAttempt, retrymongo.IndexStateSince}},
		{"retention", []retrymongo.Option{retrymongo.WithResolvedRetention(7 * 24 * time.Hour)},
			[]string{"_id_", retrymongo.IndexPendingNextAttempt, retrymongo.IndexStateSince, retrymongo.IndexResolvedTTL}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := mongotest.New(t)
			s, err := retrymongo.New[storagetest.Payload](m.Database().Collection(collName), tt.opts...)
			require.NoError(t, err)
			for range 2 { // idempotent
				require.NoError(t, s.EnsureIndexes(t.Context()))
			}
			require.Equal(t, tt.want, mongotest.IndexNames(t, m, collName))
		})
	}
}

// TestTTLIndexSpec asserts the TTL index purges only resolved items after the
// configured retention.
func TestTTLIndexSpec(t *testing.T) {
	t.Parallel()

	coll := newColl(t)
	newStore[storagetest.Payload](t, coll, retrymongo.WithResolvedRetention(7*24*time.Hour))
	cur, err := coll.Indexes().List(t.Context())
	require.NoError(t, err)
	var specs []bson.M
	require.NoError(t, cur.All(t.Context(), &specs))
	var ttl bson.M
	for _, spec := range specs {
		if spec["name"] == retrymongo.IndexResolvedTTL {
			ttl = spec
		}
	}
	require.NotNil(t, ttl)
	require.EqualValues(t, 7*24*60*60, ttl["expireAfterSeconds"])
	require.Equal(t, bson.D{{Key: "state", Value: "resolved"}}, ttl["partialFilterExpression"])
}

// TestMarkInTransactionDoesNotRetry asserts a duplicate-key failure of Mark
// inside a transaction is returned at once: the error has aborted the
// transaction, so a repeated upsert would only fail with NoSuchTransaction.
func TestMarkInTransactionDoesNotRetry(t *testing.T) {
	t.Parallel()

	m := mongotest.New(t)
	mongotest.RequireReplicaSet(t, m)
	coll := m.Database().Collection(collName)
	s := newStore[storagetest.Payload](t, coll)
	// A unique payload field makes the insert of a new key fail with a
	// duplicate key, as a lost first-writer race does.
	_, err := coll.Indexes().CreateOne(t.Context(), mongo.IndexModel{
		Keys: bson.D{{Key: "note", Value: 1}}, Options: mongoOptions.Index().SetUnique(true),
	})
	require.NoError(t, err)
	_, err = s.Mark(t.Context(), storagetest.Pending("taken", t0))
	require.NoError(t, err)

	var markErr error
	err = m.WithTransaction(t.Context(), func(ctx context.Context) error {
		_, markErr = s.Mark(ctx, storagetest.Pending("k", t0))
		return markErr
	})
	require.Error(t, err)
	require.True(t, mongo.IsDuplicateKeyError(markErr), "Mark returned %v, want the duplicate-key error", markErr)
}

// TestEnsureIndexesDoesNotDropChangedTTL asserts a changed retention is
// reported, not resolved by dropping the existing TTL index.
func TestEnsureIndexesDoesNotDropChangedTTL(t *testing.T) {
	t.Parallel()

	coll := newColl(t)
	newStore[storagetest.Payload](t, coll, retrymongo.WithResolvedRetention(time.Hour))
	changed, err := retrymongo.New[storagetest.Payload](coll, retrymongo.WithResolvedRetention(2*time.Hour))
	require.NoError(t, err)
	require.Error(t, changed.EnsureIndexes(t.Context()))

	specs, err := coll.Indexes().ListSpecifications(t.Context())
	require.NoError(t, err)
	i := slices.IndexFunc(specs, func(sp mongo.IndexSpecification) bool { return sp.Name == retrymongo.IndexResolvedTTL })
	require.GreaterOrEqual(t, i, 0, "the TTL index is kept")
	require.NotNil(t, specs[i].ExpireAfterSeconds)
	require.EqualValues(t, 60*60, *specs[i].ExpireAfterSeconds, "with its original retention")
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueueit_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/reliability/retryqueue"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/storagetest"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	retrymongo "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/mongo"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// collName is the collection every test creates in its own database.
const collName = "retry_items"

// mongoColl connects to the compose replica set and returns the test
// collection of a throwaway database, dropped on cleanup. It skips when
// MongoDB is unreachable.
func mongoColl(tb testing.TB) *mongodrv.Collection {
	tb.Helper()
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI(livebackend.MongoURI()).
		SetServerSelectionTimeout(3 * time.Second))
	require.NoError(tb, err)
	ctx, cancel := context.WithTimeout(tb.Context(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.WithoutCancel(tb.Context()))
		// The URI is not printed: an override may carry a password.
		tb.Skipf("mongodb unreachable (%v) — start it with: make integration-up", err)
	}
	db := client.Database("retryqueueit_" + strings.ToLower(rand.Text()[:12]))
	tb.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	return db.Collection(collName)
}

// newStore returns a Store over coll with its indexes ensured.
func newStore(tb testing.TB, coll *mongodrv.Collection, opts ...retrymongo.Option) *retrymongo.Store[storagetest.Payload] {
	tb.Helper()
	s, err := retrymongo.New[storagetest.Payload](coll, opts...)
	require.NoError(tb, err)
	require.NoError(tb, s.EnsureIndexes(tb.Context()))
	return s
}

// indexSpecs returns the index specifications of coll by name.
func indexSpecs(tb testing.TB, coll *mongodrv.Collection) map[string]mongodrv.IndexSpecification {
	tb.Helper()
	specs, err := coll.Indexes().ListSpecifications(tb.Context())
	require.NoError(tb, err)
	out := make(map[string]mongodrv.IndexSpecification, len(specs))
	for _, spec := range specs {
		out[spec.Name] = spec
	}
	return out
}

// TestMongoStorageContract runs the storage contract suite on MongoDB, each
// contract over its own database.
func TestMongoStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storagetest.Storage {
		return newStore(tb, mongoColl(tb))
	})
}

// TestMongoStorageContractCustomFlagFields runs the contract suite again with
// renamed flag fields and a retention, the configuration of a collection an
// earlier store wrote.
func TestMongoStorageContractCustomFlagFields(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storagetest.Storage {
		return newStore(tb, mongoColl(tb),
			retrymongo.WithFlagField("needsManualRate"), retrymongo.WithFlagReasonField("manualRateReason"),
			retrymongo.WithResolvedRetention(7*24*time.Hour))
	})
}

// TestMongoIndexes asserts EnsureIndexes creates exactly the documented index
// set, idempotently, with the due-list index partial on pending items and the
// TTL index (only with a retention, rounded up to whole seconds) partial on
// resolved items.
func TestMongoIndexes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts []retrymongo.Option
		ttl  int32 // expected expireAfterSeconds; 0 means no TTL index
	}{
		{"no retention", nil, 0},
		{"sub-second retention", []retrymongo.Option{retrymongo.WithResolvedRetention(500 * time.Millisecond)}, 1},
		{"retention rounded up to seconds", []retrymongo.Option{retrymongo.WithResolvedRetention(90*time.Second + 100*time.Millisecond)}, 91},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			coll := mongoColl(t)
			s := newStore(t, coll, tt.opts...)
			require.NoError(t, s.EnsureIndexes(t.Context()), "EnsureIndexes is idempotent")

			specs := indexSpecs(t, coll)
			want := []string{"_id_", retrymongo.IndexPendingNextAttempt, retrymongo.IndexStateSince}
			if tt.ttl > 0 {
				want = append(want, retrymongo.IndexResolvedTTL)
			}
			slices.Sort(want)
			require.Equal(t, want, slices.Sorted(maps.Keys(specs)))

			due := specs[retrymongo.IndexPendingNextAttempt]
			require.Nil(t, due.ExpireAfterSeconds)
			if tt.ttl == 0 {
				return
			}
			ttl := specs[retrymongo.IndexResolvedTTL]
			require.NotNil(t, ttl.ExpireAfterSeconds)
			require.Equal(t, tt.ttl, *ttl.ExpireAfterSeconds)
		})
	}
}

// TestMongoIndexPartialFilters reads the raw index documents: the due-list
// index covers pending items only, and the TTL index resolved items only, so
// a pending or expired item is never purged.
func TestMongoIndexPartialFilters(t *testing.T) {
	t.Parallel()
	coll := mongoColl(t)
	newStore(t, coll, retrymongo.WithResolvedRetention(time.Hour))

	cur, err := coll.Indexes().List(t.Context())
	require.NoError(t, err)
	var specs []bson.M
	require.NoError(t, cur.All(t.Context(), &specs))
	partial := make(map[string]any, len(specs))
	for _, spec := range specs {
		partial[spec["name"].(string)] = spec["partialFilterExpression"]
	}
	require.Equal(t, bson.D{{Key: "state", Value: "pending"}}, partial[retrymongo.IndexPendingNextAttempt])
	require.Equal(t, bson.D{{Key: "state", Value: "resolved"}}, partial[retrymongo.IndexResolvedTTL])
	require.Nil(t, partial[retrymongo.IndexStateSince], "the per-state index covers every state")
}

// TestMongoEnsureIndexesRejectsChangedRetention asserts a changed retention is
// reported rather than resolved by dropping the TTL index another replica
// still relies on.
func TestMongoEnsureIndexesRejectsChangedRetention(t *testing.T) {
	t.Parallel()
	coll := mongoColl(t)
	newStore(t, coll, retrymongo.WithResolvedRetention(time.Hour))

	changed, err := retrymongo.New[storagetest.Payload](coll, retrymongo.WithResolvedRetention(2*time.Hour))
	require.NoError(t, err)
	require.Error(t, changed.EnsureIndexes(t.Context()))

	ttl := indexSpecs(t, coll)[retrymongo.IndexResolvedTTL]
	require.NotNil(t, ttl.ExpireAfterSeconds, "the TTL index is kept")
	require.EqualValues(t, 3600, *ttl.ExpireAfterSeconds, "with its original retention")
}

// planStages returns every "stage" named anywhere in an explain document,
// which covers the classic and the slot-based plan layouts alike.
func planStages(v any) []string {
	var out []string
	switch v := v.(type) {
	case bson.D:
		for _, e := range v {
			if s, ok := e.Value.(string); ok && e.Key == "stage" {
				out = append(out, s)
			}
			out = append(out, planStages(e.Value)...)
		}
	case bson.A:
		for _, e := range v {
			out = append(out, planStages(e)...)
		}
	}
	return out
}

// winningStages explains a find of coll and returns the stages of its winning
// plan.
func winningStages(tb testing.TB, coll *mongodrv.Collection, filter bson.D, sort bson.D) []string {
	tb.Helper()
	var res bson.D
	err := coll.Database().RunCommand(tb.Context(), bson.D{
		{Key: "explain", Value: bson.D{
			{Key: "find", Value: coll.Name()}, {Key: "filter", Value: filter},
			{Key: "sort", Value: sort}, {Key: "limit", Value: int64(retryqueue.DefaultListLimit)},
		}},
		{Key: "verbosity", Value: "queryPlanner"},
	}).Decode(&res)
	require.NoError(tb, err)
	var planner bson.D
	for _, e := range res {
		if e.Key == "queryPlanner" {
			planner, _ = e.Value.(bson.D)
		}
	}
	require.NotNil(tb, planner, "explain has no queryPlanner: %v", res)
	for _, e := range planner {
		if e.Key == "winningPlan" {
			return planStages(e.Value)
		}
	}
	tb.Fatalf("explain has no winningPlan: %v", planner)
	return nil
}

// TestMongoListQueriesUseIndexes explains the queries ListDue and ListByState
// send (the store's README audit lists them) over a queue holding items in
// every state: each must be an index scan with its sort taken from the
// index, so a sweep stays cheap as the queue grows.
func TestMongoListQueriesUseIndexes(t *testing.T) {
	t.Parallel()
	coll := mongoColl(t)
	s := newStore(t, coll)
	t0 := storagetest.T0
	for i := range 60 {
		key := fmt.Sprintf("k%02d", i)
		_, err := s.Mark(t.Context(), storagetest.Pending(key, t0.Add(time.Duration(i)*time.Minute)))
		require.NoError(t, err)
		switch i % 3 {
		case 1:
			require.NoError(t, s.Resolve(t.Context(), key, 1, t0))
		case 2:
			expired, err := s.Expire(t.Context(), key, 1, t0, t0)
			require.NoError(t, err)
			require.True(t, expired, key)
		}
	}
	// The queries behind the API calls, as store.go builds them.
	due, err := s.ListDue(t.Context(), t0.Add(time.Hour), 0)
	require.NoError(t, err)
	require.Len(t, due, 20)
	byState, err := s.ListByState(t.Context(), retryqueue.StateExpired, 0)
	require.NoError(t, err)
	require.Len(t, byState, 20)

	queries := []struct {
		name         string
		filter, sort bson.D
	}{
		{"ListDue",
			bson.D{{Key: "state", Value: "pending"}, {Key: "nextAttemptAt", Value: bson.D{{Key: "$lte", Value: t0.Add(time.Hour)}}}},
			bson.D{{Key: "nextAttemptAt", Value: 1}, {Key: "_id", Value: 1}}},
		{"ListByState",
			bson.D{{Key: "state", Value: "expired"}},
			bson.D{{Key: "since", Value: 1}, {Key: "_id", Value: 1}}},
	}
	for _, q := range queries {
		stages := winningStages(t, coll, q.filter, q.sort)
		require.Contains(t, stages, "IXSCAN", "%s: %v", q.name, stages)
		require.NotContains(t, stages, "COLLSCAN", "%s: %v", q.name, stages)
		require.NotContains(t, stages, "SORT", "%s must take its order from the index: %v", q.name, stages)
	}
}

// TestMongoConcurrentFirstMarkAcrossReplicas races the first Mark of one key
// from several stores over one collection, each on its own client, as service
// replicas do: exactly one document is inserted and every replica gets it.
func TestMongoConcurrentFirstMarkAcrossReplicas(t *testing.T) {
	t.Parallel()
	const replicas = 8
	coll := mongoColl(t)
	newStore(t, coll)

	stores := make([]*retrymongo.Store[storagetest.Payload], replicas)
	for i := range replicas {
		client, err := mongodrv.Connect(mongoopts.Client().ApplyURI(livebackend.MongoURI()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
		replicaColl := client.Database(coll.Database().Name()).Collection(collName)
		stores[i], err = retrymongo.New[storagetest.Payload](replicaColl)
		require.NoError(t, err)
	}

	results := make([]storagetest.Item, replicas)
	errs := make([]error, replicas)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, s := range stores {
		wg.Go(func() {
			<-start
			in := storagetest.Pending("race", storagetest.T0)
			in.Payload.Note = fmt.Sprintf("replica-%d", i)
			results[i], errs[i] = s.Mark(t.Context(), in)
		})
	}
	close(start)
	wg.Wait()
	for i := range replicas {
		require.NoError(t, errs[i], "replica %d", i)
		require.Equal(t, results[0], results[i], "replica %d", i)
	}
	n, err := coll.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}

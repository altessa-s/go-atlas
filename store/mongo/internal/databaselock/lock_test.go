// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package databaselock

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mongo-migrate reads the current schema version from the document with the
// greatest _id in its collection and decodes it as a version record. A lock
// document sharing that collection would win that sort and decode as version
// 0, silently re-running every migration — the exact failure the lock exists
// to prevent.
func TestMigrationLock_UsesSeparateCollection(t *testing.T) {
	t.Parallel()

	require.NotEqual(t, "migrations", CollectionName)
}

func TestMigrationLockFilter(t *testing.T) {
	t.Parallel()

	filter := databaseLockFilter("migrations")

	require.Equal(t, "migrations", filter["_id"],
		"the filter must pin the single mutex document, otherwise the lock is not mutually exclusive")

	// The lease is compared against the server clock ($$NOW), not the
	// caller's: replicas with drifted clocks must not steal a live lock.
	require.Equal(t, bson.M{"$lte": bson.A{"$expiresAt", "$$NOW"}}, filter["$expr"])
}

func TestMigrationLockUpdate(t *testing.T) {
	t.Parallel()

	owner := bson.NewObjectID()
	pipeline := databaseLockUpdate(owner, 30*time.Second)

	require.Len(t, pipeline, 1, "update must be a single-stage pipeline")

	stage := pipeline[0]
	require.Len(t, stage, 1)
	require.Equal(t, "$set", stage[0].Key)

	set, ok := stage[0].Value.(bson.M)
	require.True(t, ok)
	require.Equal(t, owner, set["owner"])
	require.Equal(t, "$$NOW", set["acquiredAt"])

	// The stored expiry must use the requested lease and the server clock.
	require.Equal(t, bson.M{"$add": bson.A{"$$NOW", (30 * time.Second).Milliseconds()}}, set["expiresAt"])
}

// The seed document must be immediately acquirable, otherwise the first
// replica to start would wait out a full lease before migrating.
func TestMigrationLockDocument_SeedIsExpired(t *testing.T) {
	t.Parallel()

	seed := migrationLockDocument{ID: "migrations", ExpiresAt: time.Unix(0, 0).UTC()}

	raw, err := bson.Marshal(seed)
	require.NoError(t, err)

	var decoded bson.M
	require.NoError(t, bson.Unmarshal(raw, &decoded))

	require.Equal(t, "migrations", decoded["_id"])
	require.True(t, seed.ExpiresAt.Before(time.Now()))
	require.NotContains(t, decoded, "owner", "an unowned seed must not carry a zero owner")
	require.NotContains(t, decoded, "acquiredAt")
}

// The two phases have separate budgets and the outer context must cover both,
// otherwise a replica that waited for a peer would have no time left to run
// the migrations it was waiting to skip.
func TestMigrationLock_TimeoutBudgets(t *testing.T) {
	t.Parallel()

	require.Positive(t, 30*time.Second)
	require.Positive(t, 30*time.Second)
	require.Less(t, RetryBaseDelay, RetryMaxDelay)
	require.Less(t, RetryMaxDelay, 30*time.Second,
		"a single backoff step must not consume the whole wait budget")
}

func TestHold_RenewsUntilWorkReturnsPastBudget(t *testing.T) {
	t.Parallel()

	var renewals atomic.Int32
	renew := func(context.Context) error { renewals.Add(1); return nil }

	err := hold(t.Context(), renew, 30*time.Millisecond, 20*time.Millisecond, func(work context.Context, held func() bool) error {
		// The step ignores its context and outlives both the budget and several lease periods.
		<-work.Done()
		time.Sleep(120 * time.Millisecond)
		require.True(t, held())
		return nil
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, renewals.Load(), int32(3), "the heartbeat keeps renewing after the work context is canceled")
}

func TestHold_LostRenewalCancelsWork(t *testing.T) {
	t.Parallel()

	renew := func(context.Context) error { return ErrLeaseLost }
	err := hold(t.Context(), renew, 30*time.Millisecond, time.Minute, func(work context.Context, held func() bool) error {
		select {
		case <-work.Done():
		case <-time.After(5 * time.Second):
			t.Error("work context was not canceled")
		}
		require.False(t, held())
		return work.Err()
	})
	require.ErrorIs(t, err, ErrLeaseLost)
}

func TestHold_TransientRenewalErrorsTolerated(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	renew := func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("transient")
		}
		return nil
	}
	err := hold(t.Context(), renew, 60*time.Millisecond, time.Minute, func(work context.Context, held func() bool) error {
		time.Sleep(150 * time.Millisecond)
		require.True(t, held())
		require.NoError(t, work.Err())
		return nil
	})
	require.NoError(t, err)
}

func TestHold_PersistentRenewalErrorsLoseTheLease(t *testing.T) {
	t.Parallel()

	renew := func(context.Context) error { return errors.New("unreachable") }
	err := hold(t.Context(), renew, 30*time.Millisecond, time.Minute, func(work context.Context, _ func() bool) error {
		<-work.Done()
		return nil
	})
	require.ErrorIs(t, err, ErrLeaseLost)
}

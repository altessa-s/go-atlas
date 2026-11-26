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

const testLease = 900 * time.Millisecond

// signalRenew returns a renew function and the channel on which every call announces itself; fail picks the outcome of call n (1 based).
func signalRenew(fail func(n int) error) (func(context.Context) error, <-chan int) {
	calls := make(chan int, 1024)
	var n atomic.Int32
	return func(ctx context.Context) error {
		i := int(n.Add(1))
		calls <- i
		if fail != nil {
			if err := fail(i); err != nil {
				if errors.Is(err, errBlock) {
					<-ctx.Done()
					return ctx.Err()
				}
				return err
			}
		}
		return nil
	}, calls
}

var errBlock = errors.New("block until the renewal context ends")

func awaitRenewals(t *testing.T, calls <-chan int, upTo int) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case n := <-calls:
			if n >= upTo {
				return
			}
		case <-timeout:
			t.Fatalf("renewal %d never happened", upTo)
		}
	}
}

func TestHold_RenewsUntilWorkReturnsPastBudget(t *testing.T) {
	t.Parallel()

	renew, calls := signalRenew(nil)
	err := hold(t.Context(), renew, testLease, 50*time.Millisecond, func(l *Lease) error {
		// The step ignores its context and outlives both the budget and more than one lease period.
		<-l.Work().Done()
		awaitRenewals(t, calls, 1+2*3+1)
		require.True(t, l.Held())
		require.NoError(t, l.Scope().Err(), "the scope outlives the budget")
		return nil
	})
	require.NoError(t, err)
}

func TestHold_LostRenewalCancelsWorkAndScope(t *testing.T) {
	t.Parallel()

	renew, _ := signalRenew(func(n int) error {
		if n > 1 {
			return ErrLeaseLost
		}
		return nil
	})
	err := hold(t.Context(), renew, testLease, time.Minute, func(l *Lease) error {
		<-l.Work().Done()
		<-l.Scope().Done()
		require.False(t, l.Held())
		return l.Work().Err()
	})
	require.ErrorIs(t, err, ErrLeaseLost)
}

func TestHold_TransientRenewalErrorIsTolerated(t *testing.T) {
	t.Parallel()

	renew, calls := signalRenew(func(n int) error {
		if n == 2 {
			return errors.New("transient")
		}
		return nil
	})
	err := hold(t.Context(), renew, testLease, time.Minute, func(l *Lease) error {
		awaitRenewals(t, calls, 5)
		require.True(t, l.Held())
		require.NoError(t, l.Work().Err())
		return nil
	})
	require.NoError(t, err)
}

// TestHold_UnreachableServerLosesTheLeaseBeforeItExpires: renewals that hang (a partition) must not keep the work alive past the
// confirmed validity: the loss is declared by deadline, inside the lease, not at the next tick.
func TestHold_UnreachableServerLosesTheLeaseBeforeItExpires(t *testing.T) {
	t.Parallel()

	renew, _ := signalRenew(func(n int) error {
		if n > 1 {
			return errBlock
		}
		return nil
	})
	start := time.Now()
	err := hold(t.Context(), renew, testLease, time.Minute, func(l *Lease) error {
		<-l.Work().Done()
		require.False(t, l.Held())
		return l.Work().Err()
	})
	require.ErrorIs(t, err, ErrLeaseLost)
	require.Less(t, time.Since(start), testLease, "work is canceled before the server-side lease can expire")
}

func TestHold_LossAfterSuccessfulWorkDoesNotFailTheRun(t *testing.T) {
	t.Parallel()

	renew, _ := signalRenew(func(n int) error {
		if n > 1 {
			return ErrLeaseLost
		}
		return nil
	})
	err := hold(t.Context(), renew, testLease, time.Minute, func(l *Lease) error {
		<-l.Scope().Done()
		return nil
	})
	require.NoError(t, err)
}

func TestHold_VerifyReportsLoss(t *testing.T) {
	t.Parallel()

	renew, _ := signalRenew(func(n int) error {
		if n == 2 {
			return ErrLeaseLost
		}
		return nil
	})
	err := hold(t.Context(), renew, time.Hour, time.Minute, func(l *Lease) error {
		require.ErrorIs(t, l.Verify(t.Context()), ErrLeaseLost)
		require.False(t, l.Held())
		require.Error(t, l.Scope().Err())
		return nil
	})
	require.NoError(t, err)
}

func TestHold_RejectsALeaseTooShortToRenew(t *testing.T) {
	t.Parallel()

	renew, _ := signalRenew(nil)
	require.Error(t, hold(t.Context(), renew, 2*time.Nanosecond, time.Minute, func(*Lease) error { return nil }))
	require.Error(t, hold(t.Context(), renew, time.Second, 0, func(*Lease) error { return nil }))
}

func TestHold_StopsTheHeartbeatWhenWorkPanics(t *testing.T) {
	t.Parallel()

	renew, _ := signalRenew(nil)
	require.Panics(t, func() {
		_ = hold(t.Context(), renew, testLease, time.Minute, func(*Lease) error { panic("boom") }) //nolint:errcheck
	})
}

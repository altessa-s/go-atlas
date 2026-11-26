// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package databaselock

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreretry "github.com/altessa-s/go-atlas-core/retry"
)

// Shared lease collection and acquisition backoff settings.
const (
	CollectionName = "migrations_lock"
	RetryBaseDelay = 200 * time.Millisecond
	RetryMaxDelay  = 2 * time.Second
	retryJitter    = 0.2
	releaseTimeout = 5 * time.Second
	lockFieldOwner = "owner"
)

// serverNow uses the database clock for lease comparisons and timestamps.
const serverNow = "$$NOW"

// ErrHeld indicates an unexpired lease owned by another process.
var ErrHeld = errors.New("migration lock is held by another process")

// ErrLeaseLost indicates that the lease could not be renewed: another process owns it, or no renewal succeeded for a whole lease.
var ErrLeaseLost = errors.New("migration lock lease lost")

type migrationLockDocument struct {
	ID         string        `bson:"_id"`
	Owner      bson.ObjectID `bson:"owner,omitempty"`
	AcquiredAt time.Time     `bson:"acquiredAt,omitempty"`
	ExpiresAt  time.Time     `bson:"expiresAt"`
}

// Acquire takes the mutex document id of collection for owner with the given lease, blocking until it is taken, ctx
// runs out, or a non-retryable driver error occurs. The migration lock and the index reconciliation lock are
// such documents.
func Acquire(ctx context.Context, collection *mongo.Collection, id string, owner bson.ObjectID, lease time.Duration) error {
	if err := seedDatabaseLock(ctx, collection, id); err != nil {
		return coreerrs.Wrap(err, "seed lock "+id)
	}

	err := coreretry.Do(ctx, func(ctx context.Context) error {
		result, err := collection.UpdateOne(ctx, databaseLockFilter(id), databaseLockUpdate(owner, lease))
		switch {
		case err != nil:
			return err
		case result.MatchedCount == 0:
			return ErrHeld
		default:
			return nil
		}
	},
		coreretry.WithMaxAttempts(-1), // Bounded by ctx, not by an attempt count.
		coreretry.WithShouldRetry(func(err error) bool { return errors.Is(err, ErrHeld) }),
		coreretry.WithNextDelay(coreretry.Exponential(
			RetryBaseDelay,
			coreretry.WithExponentialMaxDelay(RetryMaxDelay),
			coreretry.WithExponentialJitter(retryJitter),
		)),
	)
	if err != nil {
		return coreerrs.Wrap(err, "acquire lock "+id)
	}

	return nil
}

// seedDatabaseLock inserts the mutex document id in an already-expired state.
// Seeding separately keeps the acquisition update free of an upsert, so a
// contended acquire is a plain "matched nothing" rather than a duplicate-key
// error that has to be told apart from a real one.
func seedDatabaseLock(ctx context.Context, collection *mongo.Collection, id string) error {
	_, err := collection.InsertOne(ctx, migrationLockDocument{
		ID:        id,
		ExpiresAt: time.Unix(0, 0).UTC(),
	})
	if mongo.IsDuplicateKeyError(err) {
		return nil // Already seeded by this or another process.
	}

	return err
}

// databaseLockFilter matches the mutex document id only while it is free or
// its lease has lapsed.
//
// The lease is compared against $$NOW — the server's clock — rather than the
// caller's, so replicas whose clocks have drifted apart cannot steal a live
// lock from each other.
func databaseLockFilter(id string) bson.M {
	return bson.M{
		"_id":   id,
		"$expr": bson.M{"$lte": bson.A{"$expiresAt", serverNow}},
	}
}

// databaseLockUpdate stamps ownership and a fresh lease onto a mutex
// document. It is an aggregation pipeline because only a pipeline can read
// $$NOW; a plain $set would have to embed the caller's clock.
func databaseLockUpdate(owner bson.ObjectID, lease time.Duration) mongo.Pipeline {
	return mongo.Pipeline{
		bson.D{{Key: "$set", Value: bson.M{
			lockFieldOwner: owner,
			"acquiredAt":   serverNow,
			"expiresAt":    bson.M{"$add": bson.A{serverNow, lease.Milliseconds()}},
		}}},
	}
}

// Release expires the lease of the mutex document id held by owner, on a context detached from ctx (release must
// still run when the work failed because ctx expired). The owner is part of the filter: a holder that overran its lease must not
// clear the next holder's claim.
func Release(ctx context.Context, collection *mongo.Collection, id string, owner bson.ObjectID) error {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()

	filter := bson.M{"_id": id, lockFieldOwner: owner}
	update := bson.M{"$set": bson.M{"expiresAt": time.Unix(0, 0).UTC()}, "$unset": bson.M{lockFieldOwner: ""}}
	_, err := collection.UpdateOne(releaseCtx, filter, update)
	return err
}

// Renew extends the lease of the mutex document id by lease from the server clock, while owner still holds the document. A document
// owned by someone else, or missing, is [ErrLeaseLost].
func Renew(ctx context.Context, collection *mongo.Collection, id string, owner bson.ObjectID, lease time.Duration) error {
	result, err := collection.UpdateOne(ctx, bson.M{"_id": id, lockFieldOwner: owner}, mongo.Pipeline{
		bson.D{{Key: "$set", Value: bson.M{"expiresAt": bson.M{"$add": bson.A{serverNow, lease.Milliseconds()}}}}},
	})
	switch {
	case err != nil:
		return err
	case result.MatchedCount == 0:
		return ErrLeaseLost
	default:
		return nil
	}
}

// Hold runs fn while keeping the lease of the mutex document id, taken by [Acquire] for owner, alive. The lease is a renewable
// resource separate from the work budget: a heartbeat renews it every lease/3 until fn returns, independently of the work context.
// The work context passed to fn carries the budget and is canceled when the budget ends, when ctx is canceled, or when a renewal
// fails for good (ownership lost, or no renewal succeeded for a whole lease). After the work context is canceled the heartbeat
// keeps renewing while fn still runs, so a step that ignores its context never overlaps a successor as long as renewals succeed.
// held reports whether the lease is still believed to be owned; it turns false once ownership is lost. When the lease was lost
// during fn, the result wraps [ErrLeaseLost].
func Hold(
	ctx context.Context, collection *mongo.Collection, id string, owner bson.ObjectID, lease, budget time.Duration,
	fn func(work context.Context, held func() bool) error,
) error {
	renew := func(ctx context.Context) error { return Renew(ctx, collection, id, owner, lease) }
	return hold(ctx, renew, lease, budget, fn)
}

func hold(
	ctx context.Context, renew func(context.Context) error, lease, budget time.Duration,
	fn func(work context.Context, held func() bool) error,
) error {
	interval := lease / 3
	workCtx, cancelWork := context.WithTimeout(ctx, budget)
	defer cancelWork()

	var lost atomic.Bool
	held := func() bool { return !lost.Load() }
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Renewals run detached from the work context: they must outlive its cancellation.
		renewBase := context.WithoutCancel(ctx)
		lastOK := time.Now()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			renewCtx, cancel := context.WithTimeout(renewBase, interval)
			err := renew(renewCtx)
			cancel()
			switch {
			case err == nil:
				lastOK = time.Now()
			case errors.Is(err, ErrLeaseLost) || time.Since(lastOK) >= lease:
				lost.Store(true)
				cancelWork()
				return
			}
		}
	}()

	err := fn(workCtx, held)
	close(stop)
	<-done
	if lost.Load() {
		return errors.Join(err, ErrLeaseLost)
	}
	return err
}

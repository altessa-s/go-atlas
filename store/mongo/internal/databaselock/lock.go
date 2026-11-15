// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package databaselock

import (
	"context"
	"errors"
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

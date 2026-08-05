// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
)

// Migration lock configuration constants.
const (
	// MigrationLockCollectionName is the collection holding the migration
	// mutex document. It is deliberately NOT MigrationCollectionName:
	// mongo-migrate derives the current schema version from the document with
	// the greatest _id in its own collection and decodes it as a version
	// record, so a lock document living there would read back as version 0 and
	// make every migration run again.
	MigrationLockCollectionName = "migrations_lock"

	// MigrationLockID is the _id of the single mutex document. One document
	// per database is what makes the lock mutually exclusive.
	MigrationLockID = "migrations"

	// MigrationLockWaitTimeout bounds how long a replica waits for a peer to
	// finish migrating before giving up. It is a separate budget from
	// MigrationTimeout, which bounds the migrations themselves.
	MigrationLockWaitTimeout = 30 * time.Second

	// MigrationLockRetryBaseDelay is the first delay between lock attempts.
	MigrationLockRetryBaseDelay = 200 * time.Millisecond
	// MigrationLockRetryMaxDelay caps the backoff between lock attempts.
	MigrationLockRetryMaxDelay = 2 * time.Second
	// migrationLockRetryJitter spreads simultaneously-started replicas out so
	// they do not retry in lockstep.
	migrationLockRetryJitter = 0.2

	// migrationLockReleaseTimeout bounds the best-effort release, which runs
	// on a context detached from the (possibly already expired) caller's.
	migrationLockReleaseTimeout = 5 * time.Second
)

// ErrMigrationLockHeld is returned while another process holds an unexpired
// migration lock. It drives the acquisition retry loop and is never returned
// to callers of [Mongo.migrate] — a wait that runs out of budget surfaces as
// the context error instead.
var ErrMigrationLockHeld = errors.New("migration lock is held by another process")

// migrationLockDocument is the mutex document stored in
// [MigrationLockCollectionName].
type migrationLockDocument struct {
	ID         string        `bson:"_id"`
	Owner      bson.ObjectID `bson:"owner,omitempty"`
	AcquiredAt time.Time     `bson:"acquiredAt,omitempty"`
	ExpiresAt  time.Time     `bson:"expiresAt"`
}

// withMigrationLock runs fn while holding a database-wide migration lock, so
// that concurrently starting replicas apply each migration exactly once.
//
// mongo-migrate reads the current version and writes the new one without any
// locking of its own, so two processes that start together both read version N
// and both run migration N+1. Migration bodies are arbitrary code (create a
// collection, backfill documents, rename fields) rather than idempotent DDL, so
// running one twice is not generally safe.
//
// Waiters block rather than skip: a migration is a one-shot critical section
// that everyone must observe the result of, not a role one replica owns. Once
// the holder finishes, the next waiter acquires the lock, re-reads the version
// and finds nothing left to apply.
//
// The lock is a TTL lock, which bounds the damage from a crashed holder but
// cannot fence one that is merely slow: fn runs under a context capped at
// MigrationTimeout, which is also the lock's lease, so a holder cannot still be
// working after its lease lapses.
func (m *Mongo) withMigrationLock(ctx context.Context, fn func(context.Context) error) error {
	collection := m.client.Database(m.DatabaseName()).Collection(MigrationLockCollectionName)
	owner := bson.NewObjectID()

	acquireCtx, acquireCancel := corecontext.WithMaxTimeout(ctx, MigrationLockWaitTimeout)
	defer acquireCancel()

	if err := acquireMigrationLock(acquireCtx, collection, owner); err != nil {
		return err
	}

	defer m.releaseMigrationLock(ctx, collection, owner)

	workCtx, workCancel := corecontext.WithMaxTimeout(ctx, MigrationTimeout)
	defer workCancel()

	return fn(workCtx)
}

// acquireMigrationLock blocks until the lock is taken, ctx runs out, or a
// non-retryable driver error occurs.
func acquireMigrationLock(ctx context.Context, collection *mongo.Collection, owner bson.ObjectID) error {
	if err := seedMigrationLock(ctx, collection); err != nil {
		return coreerrs.Wrap(err, "seed migration lock")
	}

	err := coreretry.Do(ctx, func(ctx context.Context) error {
		result, err := collection.UpdateOne(ctx, migrationLockFilter(), migrationLockUpdate(owner))
		switch {
		case err != nil:
			return err
		case result.MatchedCount == 0:
			return ErrMigrationLockHeld
		default:
			return nil
		}
	},
		coreretry.WithMaxAttempts(-1), // Bounded by ctx, not by an attempt count.
		coreretry.WithShouldRetry(func(err error) bool { return errors.Is(err, ErrMigrationLockHeld) }),
		coreretry.WithNextDelay(coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: MigrationLockRetryBaseDelay,
			MaxDelay:  MigrationLockRetryMaxDelay,
			Jitter:    migrationLockRetryJitter,
		})),
	)
	if err != nil {
		return coreerrs.Wrap(err, "acquire migration lock")
	}

	return nil
}

// seedMigrationLock inserts the mutex document in an already-expired state.
// Seeding separately keeps the acquisition update free of an upsert, so a
// contended acquire is a plain "matched nothing" rather than a duplicate-key
// error that has to be told apart from a real one.
func seedMigrationLock(ctx context.Context, collection *mongo.Collection) error {
	_, err := collection.InsertOne(ctx, migrationLockDocument{
		ID:        MigrationLockID,
		ExpiresAt: time.Unix(0, 0).UTC(),
	})
	if duplicate, _ := IsErrorDuplicate(err); duplicate {
		return nil // Already seeded by this or another process.
	}

	return err
}

// migrationLockFilter matches the mutex document only while it is free or its
// lease has lapsed.
//
// The lease is compared against $$NOW — the server's clock — rather than the
// caller's, so replicas whose clocks have drifted apart cannot steal a live
// lock from each other.
func migrationLockFilter() bson.M {
	return bson.M{
		"_id":   MigrationLockID,
		"$expr": bson.M{"$lte": bson.A{"$expiresAt", "$$NOW"}},
	}
}

// migrationLockUpdate stamps ownership and a fresh lease onto the mutex
// document. It is an aggregation pipeline because only a pipeline can read
// $$NOW; a plain $set would have to embed the caller's clock.
func migrationLockUpdate(owner bson.ObjectID) mongo.Pipeline {
	return mongo.Pipeline{
		bson.D{{Key: "$set", Value: bson.M{
			"owner":      owner,
			"acquiredAt": "$$NOW",
			"expiresAt":  bson.M{"$add": bson.A{"$$NOW", MigrationTimeout.Milliseconds()}},
		}}},
	}
}

// releaseMigrationLock expires the lease so the next waiter proceeds without
// sitting out the full TTL.
//
// The owner is part of the filter: if this process overran its lease and
// another already took the lock, this must not clear the new holder's claim.
// Release is best-effort — a failure only means waiters fall back to the TTL —
// so it logs rather than propagating.
func (m *Mongo) releaseMigrationLock(ctx context.Context, collection *mongo.Collection, owner bson.ObjectID) {
	// Detached from ctx: release must still run when the migration failed
	// because the caller's context expired.
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), migrationLockReleaseTimeout)
	defer cancel()

	filter := bson.M{"_id": MigrationLockID, "owner": owner}
	update := bson.M{"$set": bson.M{"expiresAt": time.Unix(0, 0).UTC()}, "$unset": bson.M{"owner": ""}}

	if _, err := collection.UpdateOne(releaseCtx, filter, update); err != nil && m.config.Logger != nil {
		m.config.Logger.Warn("failed to release migration lock",
			"error", err, "collection", MigrationLockCollectionName)
	}
}

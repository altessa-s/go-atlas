// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package databaselock

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	lockFieldID    = "_id"
	lockFieldExp   = "expiresAt"
	opSet          = "$set"
	// renewDivisor is the number of heartbeats per lease.
	renewDivisor = 3
	// marginDivisor sizes the safety margin that is subtracted from the confirmed validity of the lease.
	marginDivisor = 6
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
		lockFieldID: id,
		"$expr":     bson.M{"$lte": bson.A{"$expiresAt", serverNow}},
	}
}

// databaseLockUpdate stamps ownership and a fresh lease onto a mutex
// document. It is an aggregation pipeline because only a pipeline can read
// $$NOW; a plain $set would have to embed the caller's clock.
func databaseLockUpdate(owner bson.ObjectID, lease time.Duration) mongo.Pipeline {
	return mongo.Pipeline{
		bson.D{{Key: opSet, Value: bson.M{
			lockFieldOwner: owner,
			"acquiredAt":   serverNow,
			lockFieldExp:   bson.M{"$add": bson.A{serverNow, lease.Milliseconds()}},
		}}},
	}
}

// Release expires the lease of the mutex document id held by owner, on a context detached from ctx (release must
// still run when the work failed because ctx expired). The owner is part of the filter: a holder that overran its lease must not
// clear the next holder's claim.
func Release(ctx context.Context, collection *mongo.Collection, id string, owner bson.ObjectID) error {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()

	filter := bson.M{lockFieldID: id, lockFieldOwner: owner}
	update := bson.M{opSet: bson.M{lockFieldExp: time.Unix(0, 0).UTC()}, "$unset": bson.M{lockFieldOwner: ""}}
	_, err := collection.UpdateOne(releaseCtx, filter, update)
	return err
}

// Renew extends the lease of the mutex document id by lease from the server clock, while owner still holds it and the lease has not
// lapsed. A document owned by someone else, missing, or already expired is [ErrLeaseLost].
func Renew(ctx context.Context, collection *mongo.Collection, id string, owner bson.ObjectID, lease time.Duration) error {
	filter := bson.M{lockFieldID: id, lockFieldOwner: owner, "$expr": bson.M{"$gt": bson.A{"$expiresAt", serverNow}}}
	result, err := collection.UpdateOne(ctx, filter, mongo.Pipeline{
		bson.D{{Key: opSet, Value: bson.M{lockFieldExp: bson.M{"$add": bson.A{serverNow, lease.Milliseconds()}}}}},
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

// Lease is the view fn gets of a lease kept alive by [Hold].
type Lease struct {
	work, scope context.Context
	h           *holder
}

// Work is the context of the work: it carries the budget and is canceled when the budget ends, when the caller's context is
// canceled, or when the lease is lost.
func (l *Lease) Work() context.Context { return l.work }

// Scope is a context detached from the budget and from the caller's cancellation, canceled only when the lease is lost (or Hold
// returns). Work that must outlive the budget but never the lease, such as recording a result or a rollback, derives from it.
func (l *Lease) Scope() context.Context { return l.scope }

// Held reports whether the lease is still owned with a safety margin: the last successful renewal was sent less than a lease minus
// the margin ago, and no renewal found the lease taken.
func (l *Lease) Held() bool { return l.h.held() }

// Verify asks the server whether this process still owns the lease, extending it when it does. It returns [ErrLeaseLost] otherwise,
// or when the margin already lapsed.
func (l *Lease) Verify(ctx context.Context) error {
	remaining := l.h.remaining()
	if remaining <= 0 || !l.h.held() {
		l.h.lose()
		return ErrLeaseLost
	}
	return l.h.attempt(ctx, remaining)
}

// holder tracks the confirmed validity of a lease on the local monotonic clock.
type holder struct {
	renew  func(context.Context) error
	lease  time.Duration
	margin time.Duration
	onLose func()

	mu         sync.Mutex
	validUntil time.Time
	lost       bool
}

func (h *holder) held() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.lost && time.Now().Before(h.validUntil)
}

func (h *holder) remaining() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return time.Until(h.validUntil)
}

func (h *holder) lose() {
	h.mu.Lock()
	already := h.lost
	h.lost = true
	h.mu.Unlock()
	if !already {
		h.onLose()
	}
}

// attempt sends one renewal bounded by bound. Validity is counted from the moment the request was sent, never from the response: the
// server stamps the expiry no later than the response arrives.
func (h *holder) attempt(ctx context.Context, bound time.Duration) error {
	sent := time.Now()
	renewCtx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	err := h.renew(renewCtx)
	switch {
	case err == nil:
		h.mu.Lock()
		if until := sent.Add(h.lease - h.margin); !h.lost && until.After(h.validUntil) {
			h.validUntil = until
		}
		h.mu.Unlock()
	case errors.Is(err, ErrLeaseLost):
		h.lose()
	}
	return err
}

func (h *holder) heartbeat(ctx context.Context) {
	interval := h.lease / renewDivisor
	next := time.Now().Add(interval)
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		wake := next
		if until := time.Now().Add(h.remaining()); until.Before(wake) {
			wake = until
		}
		timer.Reset(time.Until(wake))
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		now := time.Now()
		remaining := h.remaining()
		if remaining <= 0 {
			h.lose()
			return
		}
		if now.Before(next) {
			continue
		}
		next = now.Add(interval)
		_ = h.attempt(ctx, remaining) //nolint:errcheck // the outcome is recorded in the holder
		h.mu.Lock()
		lost := h.lost
		h.mu.Unlock()
		if lost {
			return
		}
	}
}

// Hold runs fn while keeping the lease of the mutex document id, taken by [Acquire] for owner, alive. The lease is a renewable
// resource separate from the work budget: ownership is confirmed once up front, then a heartbeat renews it every lease/3 until fn
// returns, independently of the work context. Validity is counted from the send time of the last successful renewal minus a safety
// margin (lease/6); when it lapses, or a renewal finds the lease taken, the lease is lost and the work and scope contexts of l are
// canceled at that moment. Each renewal is bounded by the remaining validity and canceled when fn returns. A step that ignores its
// context therefore never overlaps a successor as long as renewals succeed. When fn fails and the lease was lost meanwhile, the
// result wraps [ErrLeaseLost]; when fn succeeded its result stands.
func Hold(
	ctx context.Context, collection *mongo.Collection, id string, owner bson.ObjectID, lease, budget time.Duration,
	fn func(l *Lease) error,
) error {
	renew := func(ctx context.Context) error { return Renew(ctx, collection, id, owner, lease) }
	return hold(ctx, renew, lease, budget, fn)
}

func hold(ctx context.Context, renew func(context.Context) error, lease, budget time.Duration, fn func(l *Lease) error) error {
	if lease/renewDivisor <= 0 || budget <= 0 {
		return fmt.Errorf("databaselock: lease %v and budget %v must be positive and at least %d ns", lease, budget, renewDivisor)
	}
	detached := context.WithoutCancel(ctx)
	scope, cancelScope := context.WithCancel(detached)
	defer cancelScope()
	work, cancelWork := context.WithTimeout(ctx, budget)
	defer cancelWork()
	h := &holder{renew: renew, lease: lease, margin: lease / marginDivisor, onLose: func() { cancelScope(); cancelWork() }}

	// Confirm ownership before any work: the validity clock starts at this send.
	if err := h.attempt(scope, lease); err != nil {
		return err
	}

	heartbeatCtx, stopHeartbeat := context.WithCancel(detached)
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.heartbeat(heartbeatCtx)
	}()
	defer func() {
		stopHeartbeat()
		<-done
	}()

	err := fn(&Lease{work: work, scope: scope, h: h})
	if err == nil {
		return nil
	}
	h.mu.Lock()
	lost := h.lost
	h.mu.Unlock()
	if lost {
		return errors.Join(err, ErrLeaseLost)
	}
	return err
}

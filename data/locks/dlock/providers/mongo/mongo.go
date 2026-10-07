// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/internal/leasing"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// serverNow is the MongoDB aggregation variable holding the server's current
// time. Every lease decision uses it, so replicas with skewed clocks agree on
// whether a lease is still valid.
const serverNow = "$$NOW"

// lockDoc is the stored state of one lock key. The document outlives the
// leases taken on it: releasing a lock only ends its lease, so fencing keeps
// growing across holders instead of restarting when a document is removed.
type lockDoc struct {
	Key        string    `bson:"_id"`
	Owner      string    `bson:"owner"`
	Fencing    int64     `bson:"fencing"`
	AcquiredAt time.Time `bson:"acquired_at"`
	RenewedAt  time.Time `bson:"renewed_at"`
	ExpiresAt  time.Time `bson:"expires_at"`
	TTLMillis  int64     `bson:"ttl_ms"`
}

// Locker is a [providers.Provider] keeping each lock as a leased document in
// a MongoDB collection. A lock is taken by one conditional upsert that
// succeeds only while the key has no unexpired lease, renewed in the
// background, and released when its context ends or Release is called. Lease
// expiry is judged by the server clock. The fencing token grows by one with
// every acquisition of a key.
//
// The collection needs no index besides _id. Lock documents are kept after
// release (one per key ever locked) so fencing tokens stay monotonic. Every
// lock write uses majority write concern whatever the database handle
// carries: a lease acknowledged by a primary that then rolls it back in a
// failover would let a second holder receive the same fencing token. Lock
// decisions are made by those writes alone. Reads go to the primary whatever
// the handle's read preference, so GetLockInfo sees this client's own lease
// writes (a secondary may lag them); they keep the handle's read concern,
// since a majority read without a causally consistent session may not yet
// see such a write either.
type Locker struct {
	s      *store
	engine *leasing.Engine
}

// store implements [leasing.Store] on the locks collection.
type store struct {
	coll *mongodrv.Collection
	ttl  time.Duration
}

var (
	_ providers.Provider = (*Locker)(nil)
	_ providers.Prober   = (*Locker)(nil)
	_ leasing.Store      = (*store)(nil)
)

// New creates a MongoDB lock provider on db. It performs no I/O.
func New(db *mongodrv.Database, opts ...Option) (*Locker, error) {
	if db == nil {
		return nil, errors.New("mongo dlock: database is required")
	}
	o := newOptions(opts...)
	// The lease is stored in whole milliseconds, so every deadline uses the
	// TTL truncated the same way; the renewal must come first.
	ttl, interval, err := leasing.Timing("mongo dlock", o.ttl, o.renewRatio)
	if err != nil {
		return nil, err
	}
	coll := db.Collection(o.collection, mongoopts.Collection().
		SetWriteConcern(writeconcern.Majority()).
		SetReadPreference(readpref.Primary()))
	s := &store{coll: coll, ttl: ttl}
	return &Locker{s: s, engine: leasing.New(s, leasing.Config{
		Logger: o.logger, TTL: ttl, Interval: interval, OperationsTimeout: o.operationsTimeout,
	})}, nil
}

// Lock makes a single attempt to take the lock for key and returns
// [errs.ErrLockNotHeld] when another holder has an unexpired lease on it.
//
// ctx scopes the lock: while it lives the lease is renewed every
// TTL × renew ratio; when it ends the lease is released. A lease that cannot
// be renewed because another holder took the key over is reported lost (see
// [providers.Lock.GetLockInfo]); one that fails transiently is retried on the
// next renewal. Lock operations never join a MongoDB session or transaction
// the caller's context carries.
func (l *Locker) Lock(ctx context.Context, key string) (providers.Lock, error) {
	return l.engine.Lock(ctx, key)
}

// GetLockInfo returns the lease currently held on key, or
// [errs.ErrLockNotHeld] when the key has no unexpired lease.
func (l *Locker) GetLockInfo(ctx context.Context, key string) (*providers.LockInfo, error) {
	return l.engine.GetLockInfo(ctx, key)
}

// Close stops renewing every lock held through this provider, rejects
// further Lock calls, waits for acquisitions already in flight and releases
// every held lock. It returns the release failures; calling Close again
// retries the cleanup of what is still held (as does each lock's Release).
// The MongoDB client is managed by the caller.
func (l *Locker) Close(ctx context.Context) error {
	return l.engine.Close(ctx)
}

// Probe implements [providers.Prober]: it fails when the provider is closed
// or the MongoDB deployment is unreachable.
func (l *Locker) Probe(ctx context.Context) error {
	return l.engine.Probe(ctx)
}

// Acquire implements [leasing.Store]: it takes the lease of key for owner
// when the key has no unexpired lease, creating the document on the key's
// first use. It is one atomic upsert matched on _id alone (MongoDB rejects
// $expr in an upsert filter); every field is set through $cond on whether the
// stored lease expired, so a held key is rewritten with its own values and
// the returned owner tells whether this caller won.
func (s *store) Acquire(ctx context.Context, key, owner string) (uint64, bool, error) {
	ctx = isolated(ctx)
	ttl := s.ttl.Milliseconds()
	free := bson.M{"$lte": bson.A{bson.M{"$ifNull": bson.A{"$expires_at", nil}}, serverNow}}
	take := func(taken, kept any) bson.M { return bson.M{"$cond": bson.A{free, taken, kept}} }
	update := mongodrv.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		"owner":       take(owner, "$owner"),
		"fencing":     take(bson.M{"$add": bson.A{bson.M{"$ifNull": bson.A{"$fencing", 0}}, 1}}, "$fencing"),
		"acquired_at": take(serverNow, "$acquired_at"),
		"renewed_at":  take(serverNow, "$renewed_at"),
		"expires_at":  take(bson.M{"$add": bson.A{serverNow, ttl}}, "$expires_at"),
		"ttl_ms":      take(ttl, "$ttl_ms"),
	}}}}
	var doc lockDoc
	err := s.coll.FindOneAndUpdate(ctx, bson.M{"_id": key}, update,
		mongoopts.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(mongoopts.After).
			SetProjection(bson.M{"_id": 0, "owner": 1, "fencing": 1})).Decode(&doc)
	switch {
	case mongodrv.IsDuplicateKeyError(err):
		// Two first uses of the key raced to insert it; the other won.
		return 0, false, nil
	case err != nil:
		return 0, false, coreerrs.WrapOperation(err, "acquire MongoDB lock")
	case doc.Owner != owner:
		return 0, false, nil
	}
	return uint64(doc.Fencing), true, nil //nolint:gosec // fencing starts at 1 and only grows
}

// Read implements [leasing.Store].
func (s *store) Read(ctx context.Context, key string) (*providers.LockInfo, error) {
	return s.readLease(isolated(ctx), bson.M{"_id": key})
}

// ReadOwned implements [leasing.Store].
func (s *store) ReadOwned(ctx context.Context, key, owner string, fencing uint64) (*providers.LockInfo, error) {
	return s.readLease(isolated(ctx), ownedFilter(key, owner, fencing))
}

// readLease returns the unexpired lease matching filter, or
// [errs.ErrLockNotHeld].
func (s *store) readLease(ctx context.Context, filter bson.M) (*providers.LockInfo, error) {
	filter["$expr"] = bson.M{"$gt": bson.A{"$expires_at", serverNow}}
	var doc lockDoc
	err := s.coll.FindOne(ctx, filter).Decode(&doc)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return nil, errs.ErrLockNotHeld
	}
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read MongoDB lock")
	}
	ttl := time.Duration(doc.TTLMillis) * time.Millisecond
	return &providers.LockInfo{
		Key:          doc.Key,
		Owner:        doc.Owner,
		AcquiredAt:   doc.AcquiredAt,
		LastRenewed:  doc.RenewedAt,
		TTL:          ttl,
		FencingToken: uint64(doc.Fencing), //nolint:gosec // fencing starts at 1 and only grows
		IsStale:      time.Since(doc.RenewedAt) > ttl,
	}, nil
}

// Renew implements [leasing.Store]. Only an unexpired lease is renewed: an
// expired one stays lost even when nobody took it over. The update's filter
// is evaluated against $$NOW when the write applies.
func (s *store) Renew(ctx context.Context, key, owner string, fencing uint64) (bool, error) {
	filter := ownedFilter(key, owner, fencing)
	filter["$expr"] = bson.M{"$gt": bson.A{"$expires_at", serverNow}}
	res, err := s.coll.UpdateOne(isolated(ctx), filter, mongodrv.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		"renewed_at": serverNow,
		"expires_at": bson.M{"$add": bson.A{serverNow, s.ttl.Milliseconds()}},
	}}}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// Release implements [leasing.Store]: it ends the lease while it is still
// owner's (and fencing's, when known), keeping the document.
func (s *store) Release(ctx context.Context, key, owner string, fencing uint64) error {
	_, err := s.coll.UpdateOne(isolated(ctx), ownedFilter(key, owner, fencing), mongodrv.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		"owner":      "",
		"expires_at": serverNow,
	}}}})
	return coreerrs.WrapOperation(err, "release MongoDB lock")
}

// Ping implements [leasing.Store]. Lock writes need a primary, so a reachable
// secondary does not count.
func (s *store) Ping(ctx context.Context) error {
	return coreerrs.Wrap(s.coll.Database().Client().Ping(ctx, readpref.Primary()), "mongodb unreachable")
}

// ownedFilter matches the key only while the lease is the one owner took. The
// owner id is unique to one acquisition; the fencing token, when known, is
// matched as well.
func ownedFilter(key, owner string, fencing uint64) bson.M {
	filter := bson.M{"_id": key, "owner": owner}
	if fencing != 0 {
		filter["fencing"] = int64(fencing) //nolint:gosec // fencing starts at 1 and only grows
	}
	return filter
}

// isolated returns ctx with any MongoDB session it carries hidden, so lock
// operations never join a caller's session or transaction; cancellation,
// deadline and values are kept.
func isolated(ctx context.Context) context.Context {
	return mongodrv.NewSessionContext(ctx, nil)
}

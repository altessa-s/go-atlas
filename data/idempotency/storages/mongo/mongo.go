// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	datamongo "github.com/altessa-s/go-atlas/data/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Document fields.
const (
	fieldID        = "_id"
	fieldValue     = "val"
	fieldExpiresAt = "expires_at"
	// fieldAttempt holds a marker unique to the AttemptLock call that wrote
	// the value, so acquisition never depends on two callers' bytes
	// differing.
	fieldAttempt = "attempt"
)

// Storage is a MongoDB-backed idempotency key store. Each key is one
// document {_id: key, val: <state bytes>, expires_at: <date>}; a TTL index
// on expires_at removes expired keys, and every operation also treats an
// expired document as absent, since the TTL monitor runs only once a
// minute. Writes compare and set on the stored bytes, as the memory and
// Redis backends do. Expiry is computed from the server clock ($$NOW).
//
// Every operation runs on the context it is given, so inside a MongoDB
// transaction (mongo.Session.WithTransaction) the lock and the result
// commit or abort together with the caller's own writes.
type Storage struct {
	collection *mongo.Collection
	opts       *options
}

var _ storages.Storage = (*Storage)(nil)

// New creates a Storage on the named collection of db and creates its TTL
// index. It returns an error if index creation fails.
//
// Example:
//
//	storage, err := mongo.New(db, mongo.WithTTL(24*time.Hour))
func New(db *mongo.Database, opt ...Option) (*Storage, error) {
	opts := newOptions(opt...)
	return newStorage(db.Collection(opts.collectionName), opts)
}

// NewWithCollection creates a Storage on an existing collection handle and
// creates its TTL index. WithCollectionName has no effect.
func NewWithCollection(col *mongo.Collection, opt ...Option) (*Storage, error) {
	return newStorage(col, newOptions(opt...))
}

func newStorage(col *mongo.Collection, opts *options) (*Storage, error) {
	s := &Storage{collection: col, opts: opts}
	ctx, cancel := corecontext.ApplyTimeout(corecontext.OrBackground(opts.ctx), opts.indexTimeout)
	defer cancel()
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: fieldExpiresAt, Value: 1}},
		Options: mongoOptions.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return nil, coreerrs.WrapOperationWithContext(err, "create MongoDB TTL index for idempotency keys", "collection '"+col.Name()+"'")
	}
	return s, nil
}

// document is the stored shape.
type document struct {
	Value   []byte        `bson:"val"`
	Attempt bson.ObjectID `bson:"attempt"`
}

// expiresIn returns the aggregation expression for "ttl from now".
func expiresIn(ttl time.Duration) bson.D {
	return bson.D{{Key: "$add", Value: bson.A{"$$NOW", ttl.Milliseconds()}}}
}

// live matches a document that has not expired.
var live = bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{"$" + fieldExpiresAt, "$$NOW"}}}}}

// AttemptLock tries to acquire a lock for the given key using the
// backend's configured TTL.
func (s *Storage) AttemptLock(ctx context.Context, key string, val []byte) (bool, []byte, []byte, error) {
	return s.AttemptLockWithTTL(ctx, key, val, 0)
}

// AttemptLockWithTTL writes val for key unless a live document holds the
// key, in one atomic upsert, and reports which value the key holds. lockTtl
// overrides the configured TTL when positive.
func (s *Storage) AttemptLockWithTTL(ctx context.Context, key string, val []byte, lockTtl time.Duration) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}
	ttl := s.opts.ttl
	if lockTtl > 0 {
		ttl = lockTtl
	}

	// Take the key when it is missing or expired; otherwise keep it as is.
	free := bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$type", Value: "$" + fieldValue}}, "missing"}}},
		bson.D{{Key: "$lte", Value: bson.A{"$" + fieldExpiresAt, "$$NOW"}}},
	}}}
	attempt := bson.NewObjectID()
	update := mongo.Pipeline{{{Key: "$set", Value: bson.D{
		{Key: fieldValue, Value: bson.D{{Key: "$cond", Value: bson.A{free, bson.D{{Key: "$literal", Value: val}}, "$" + fieldValue}}}},
		{Key: fieldExpiresAt, Value: bson.D{{Key: "$cond", Value: bson.A{free, expiresIn(ttl), "$" + fieldExpiresAt}}}},
		{Key: fieldAttempt, Value: bson.D{{Key: "$cond", Value: bson.A{free, attempt, "$" + fieldAttempt}}}},
	}}}}
	opts := mongoOptions.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(mongoOptions.After)

	var doc document
	err := s.collection.FindOneAndUpdate(ctx, bson.D{{Key: fieldID, Value: key}}, update, opts).Decode(&doc)
	if dup, _ := datamongo.IsErrorDuplicate(err); dup && mongo.SessionFromContext(ctx) == nil {
		// A concurrent upsert inserted the key first; read it once more.
		// Not inside a session: a failed write aborts a transaction, which
		// only a retry of the whole transaction can recover.
		err = s.collection.FindOneAndUpdate(ctx, bson.D{{Key: fieldID, Value: key}}, update, opts).Decode(&doc)
	}
	if err != nil {
		return false, nil, nil, coreerrs.WrapOperation(err, "attempt lock in MongoDB")
	}
	if doc.Attempt == attempt {
		return true, nil, slices.Clone(val), nil
	}
	return false, doc.Value, nil, nil
}

// Complete replaces the value of key with val while it still holds
// lockToken and has not expired, resetting its TTL. Returns
// [storages.ErrLockStolen] otherwise. A nil lockToken overwrites
// unconditionally, as the other backends do.
func (s *Storage) Complete(ctx context.Context, key string, val []byte, lockToken []byte) error {
	if key == "" {
		return storages.ErrEmptyKey
	}
	if lockToken == nil {
		set := mongo.Pipeline{{{Key: "$set", Value: bson.D{
			{Key: fieldValue, Value: bson.D{{Key: "$literal", Value: val}}},
			{Key: fieldExpiresAt, Value: expiresIn(s.opts.ttl)},
		}}}}
		_, err := s.collection.UpdateOne(ctx, bson.D{{Key: fieldID, Value: key}}, set, mongoOptions.UpdateOne().SetUpsert(true))
		if err != nil {
			return coreerrs.WrapOperation(err, "complete idempotency key in MongoDB")
		}
		return nil
	}
	return s.compareAndSet(ctx, key, lockToken, val, "complete idempotency key in MongoDB")
}

// Steal replaces the value of key with newVal while it still holds
// expectedVal and has not expired. Returns the new lock token, or
// [storages.ErrLockStolen] when the value changed.
func (s *Storage) Steal(ctx context.Context, key string, expectedVal, newVal []byte) ([]byte, error) {
	if key == "" {
		return nil, storages.ErrEmptyKey
	}
	if err := s.compareAndSet(ctx, key, expectedVal, newVal, "steal idempotency key in MongoDB"); err != nil {
		return nil, err
	}
	return slices.Clone(newVal), nil
}

// compareAndSet writes val for a live key holding expected, with the
// configured TTL.
func (s *Storage) compareAndSet(ctx context.Context, key string, expected, val []byte, op string) error {
	filter := bson.D{{Key: fieldID, Value: key}, {Key: fieldValue, Value: expected}}
	filter = append(filter, live...)
	set := mongo.Pipeline{{{Key: "$set", Value: bson.D{
		{Key: fieldValue, Value: bson.D{{Key: "$literal", Value: val}}},
		{Key: fieldExpiresAt, Value: expiresIn(s.opts.ttl)},
	}}}}
	res, err := s.collection.UpdateOne(ctx, filter, set)
	if err != nil {
		return coreerrs.WrapOperation(err, op)
	}
	if res.MatchedCount == 0 {
		return storages.ErrLockStolen
	}
	return nil
}

// Release deletes key only while it holds lockToken and has not expired.
// Returns
// [storages.ErrLockStolen] when it does not, [storages.ErrMissingLockState]
// for an empty token.
func (s *Storage) Release(ctx context.Context, key string, lockToken []byte) error {
	if key == "" {
		return storages.ErrEmptyKey
	}
	if len(lockToken) == 0 {
		return storages.ErrMissingLockState
	}
	// An expired lock is no longer the caller's, as on the other backends.
	filter := append(bson.D{{Key: fieldID, Value: key}, {Key: fieldValue, Value: lockToken}}, live...)
	res, err := s.collection.DeleteOne(ctx, filter)
	if err != nil {
		return coreerrs.WrapOperation(err, "release idempotency key in MongoDB")
	}
	if res.DeletedCount == 0 {
		return storages.ErrLockStolen
	}
	return nil
}

// Delete unconditionally removes key.
func (s *Storage) Delete(ctx context.Context, key string) error {
	if key == "" {
		return storages.ErrEmptyKey
	}
	if _, err := s.collection.DeleteOne(ctx, bson.D{{Key: fieldID, Value: key}}); err != nil {
		return coreerrs.WrapOperation(err, "delete idempotency key from MongoDB")
	}
	return nil
}

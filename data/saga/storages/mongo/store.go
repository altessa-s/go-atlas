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

	"github.com/altessa-s/go-atlas/data/saga"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	datamongo "github.com/altessa-s/go-atlas/data/mongo"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoDB field names for the saga instances collection.
const (
	collectionFieldId       = "_id"
	collectionFieldStatus   = "status"
	collectionFieldDeadline = "deadline"
	collectionFieldVersion  = "version"
)

// Store is a durable [saga.Store] backed by a MongoDB collection. Each saga
// instance is one document keyed by its ID; the document's monotonically
// increasing version field is the optimistic-concurrency token
// ([saga.Instance.Version]), so two coordinators cannot advance the same
// instance — the loser's Update fails with [sagaerrs.ErrVersionConflict].
type Store struct {
	collection     *mongo.Collection
	collectionName string
	ctx            context.Context
	indexTimeout   time.Duration
}

var _ saga.Store = (*Store)(nil)

// New creates a Store on the named collection of db, creating the required
// indexes. It returns an error if index creation fails.
//
// Example:
//
//	store, err := mongo.New(db, mongo.WithCollectionName("saga_instances"))
func New(db *mongo.Database, opt ...Option) (*Store, error) {
	opts := newOptions(opt...)
	s := &Store{
		collectionName: opts.collectionName,
		ctx:            corecontext.OrBackground(opts.ctx),
		indexTimeout:   opts.indexTimeout,
	}
	s.collection = db.Collection(s.collectionName)

	if err := s.createIndexes(); err != nil {
		return nil, coreerrs.WrapOperationWithContext(err, "create MongoDB indexes for saga store", "collection '"+s.collectionName+"'")
	}
	return s, nil
}

// NewWithCollection creates a Store from an existing collection handle, applying
// the context and index-timeout options. WithCollectionName has no effect (the
// provided collection is always used).
func NewWithCollection(col *mongo.Collection, opt ...Option) (*Store, error) {
	opts := newOptions(opt...)
	s := &Store{
		collection:     col,
		collectionName: col.Name(),
		ctx:            corecontext.OrBackground(opts.ctx),
		indexTimeout:   opts.indexTimeout,
	}

	if err := s.createIndexes(); err != nil {
		return nil, coreerrs.WrapOperationWithContext(err, "create MongoDB indexes for saga store", "provided collection '"+s.collectionName+"'")
	}
	return s, nil
}

// Create inserts a new instance, returning [sagaerrs.ErrInstanceExists] when a
// document with the same ID already exists.
func (s *Store) Create(ctx context.Context, inst *saga.Instance) error {
	if _, err := s.collection.InsertOne(ctx, toDocument(inst)); err != nil {
		if dup, _ := datamongo.IsErrorDuplicate(err); dup {
			return sagaerrs.ErrInstanceExists
		}
		return coreerrs.WrapOperation(err, "create saga instance in MongoDB")
	}
	return nil
}

// Get loads an instance by ID, returning [sagaerrs.ErrInstanceNotFound] when
// absent.
func (s *Store) Get(ctx context.Context, id string) (*saga.Instance, error) {
	var doc instance
	if err := s.collection.FindOne(ctx, bson.D{{Key: collectionFieldId, Value: id}}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, sagaerrs.ErrInstanceNotFound
		}
		return nil, coreerrs.WrapOperation(err, "get saga instance from MongoDB")
	}
	return fromDocument(&doc), nil
}

// Update overwrites the instance using a version-checked write. A stale
// inst.Version (another coordinator advanced the instance) yields
// [sagaerrs.ErrVersionConflict]; a missing instance yields
// [sagaerrs.ErrInstanceNotFound]. The new version is written back into
// inst.Version on success.
func (s *Store) Update(ctx context.Context, inst *saga.Instance) error {
	newVersion := inst.Version + 1
	doc := toDocument(inst)

	filter := bson.D{
		{Key: collectionFieldId, Value: inst.ID},
		{Key: collectionFieldVersion, Value: inst.Version},
	}
	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: collectionFieldStatus, Value: doc.Status},
		{Key: "stage", Value: doc.Stage},
		{Key: "data", Value: doc.Data},
		{Key: "steps", Value: doc.Steps},
		{Key: "updated_at", Value: doc.UpdatedAt},
		{Key: collectionFieldDeadline, Value: doc.Deadline},
		{Key: "last_error", Value: doc.LastError},
		{Key: collectionFieldVersion, Value: newVersion},
	}}}

	res, err := s.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return coreerrs.WrapOperation(err, "update saga instance in MongoDB")
	}
	if res.MatchedCount == 0 {
		// No document matched the {id, version} filter: either the instance is
		// gone, or its version moved on. Disambiguate so the caller sees
		// ErrInstanceNotFound vs ErrVersionConflict.
		n, cErr := s.collection.CountDocuments(ctx, bson.D{{Key: collectionFieldId, Value: inst.ID}})
		if cErr != nil {
			return coreerrs.WrapOperation(cErr, "disambiguate saga update in MongoDB")
		}
		if n == 0 {
			return sagaerrs.ErrInstanceNotFound
		}
		return sagaerrs.ErrVersionConflict
	}

	inst.Version = newVersion
	return nil
}

// FetchRecoverable returns up to limit non-terminal instances that are
// mid-compensation or past their deadline. A non-positive limit means no cap.
func (s *Store) FetchRecoverable(ctx context.Context, now time.Time, limit int) ([]*saga.Instance, error) {
	filter := bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: collectionFieldStatus, Value: string(saga.StatusCompensating)}},
		bson.D{
			{Key: collectionFieldStatus, Value: string(saga.StatusRunning)},
			{Key: collectionFieldDeadline, Value: bson.D{{Key: "$gt", Value: int64(0)}, {Key: "$lte", Value: now.Unix()}}},
		},
	}}}

	findOpts := mongoOptions.Find()
	if limit > 0 {
		findOpts.SetLimit(int64(limit))
	}

	cursor, err := s.collection.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "fetch recoverable saga instances from MongoDB")
	}
	defer func() { _ = cursor.Close(ctx) }()

	var docs []instance
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, coreerrs.WrapOperation(err, "decode recoverable saga instances from MongoDB")
	}

	out := make([]*saga.Instance, len(docs))
	for i := range docs {
		out[i] = fromDocument(&docs[i])
	}
	return out, nil
}

// Delete removes an instance. Deleting a missing instance is not an error.
func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.collection.DeleteOne(ctx, bson.D{{Key: collectionFieldId, Value: id}}); err != nil {
		return coreerrs.WrapOperation(err, "delete saga instance from MongoDB")
	}
	return nil
}

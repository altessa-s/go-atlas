// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"maps"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/reliability/saga"
	"github.com/altessa-s/go-atlas/store/mongo/cas"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	datamongo "github.com/altessa-s/go-atlas/store/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// opIn is the MongoDB set-membership operator.
const opIn = "$in"

// MongoDB field names for the saga instances collection.
const (
	collectionFieldId         = "_id"
	collectionFieldDefinition = "definition"
	collectionFieldStatus     = "status"
	collectionFieldDeadline   = "deadline"
	collectionFieldVersion    = "version"
	collectionFieldLeaseUntil = "lease_until"
	collectionFieldUpdatedAt  = "updated_at"
)

// Store is a durable [saga.Storage] backed by a MongoDB collection. Each saga
// instance is one document keyed by its ID; the document's monotonically
// increasing version field is the optimistic-concurrency token
// ([saga.Instance.Version]), so two coordinators cannot advance the same
// instance — the loser's Update fails with [saga.ErrVersionConflict].
type Store struct {
	collection     *mongo.Collection
	collectionName string
	ctx            context.Context
	indexTimeout   time.Duration
}

var _ saga.Storage = (*Store)(nil)

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

// Create inserts a new instance, returning [saga.ErrInstanceExists] when a
// document with the same ID already exists.
func (s *Store) Create(ctx context.Context, inst *saga.Instance) error {
	if _, err := s.collection.InsertOne(ctx, toDocument(inst)); err != nil {
		if dup, _ := datamongo.IsErrorDuplicate(err); dup {
			return saga.ErrInstanceExists
		}
		return coreerrs.WrapOperation(err, "create saga instance in MongoDB")
	}
	return nil
}

// Get loads an instance by ID, returning [saga.ErrInstanceNotFound] when
// absent.
func (s *Store) Get(ctx context.Context, id string) (*saga.Instance, error) {
	var doc instance
	if err := s.collection.FindOne(ctx, bson.D{{Key: collectionFieldId, Value: id}}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, saga.ErrInstanceNotFound
		}
		return nil, coreerrs.WrapOperation(err, "get saga instance from MongoDB")
	}
	return fromDocument(&doc), nil
}

// Update overwrites the instance using a version-checked write. A stale
// inst.Version (another coordinator advanced the instance) yields
// [saga.ErrVersionConflict]; a missing instance yields
// [saga.ErrInstanceNotFound]. The new version is written back into
// inst.Version on success.
func (s *Store) Update(ctx context.Context, inst *saga.Instance) error {
	newVersion := inst.Version + 1
	doc := toDocument(inst)

	filter := cas.Filter(collectionFieldId, inst.ID, collectionFieldVersion, inst.Version)
	update := cas.Inc(collectionFieldVersion)
	update["$set"] = bson.D{
		{Key: "definition", Value: doc.Definition},
		{Key: collectionFieldStatus, Value: doc.Status},
		{Key: "stage", Value: doc.Stage},
		{Key: "pending_steps", Value: doc.PendingSteps},
		{Key: "lease_owner", Value: doc.LeaseOwner},
		{Key: collectionFieldLeaseUntil, Value: doc.LeaseUntil},
		{Key: "data", Value: doc.Data},
		{Key: "steps", Value: doc.Steps},
		{Key: "created_at", Value: doc.CreatedAt},
		{Key: "updated_at", Value: doc.UpdatedAt},
		{Key: collectionFieldDeadline, Value: doc.Deadline},
		{Key: "last_error", Value: doc.LastError},
	}

	res, err := s.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return coreerrs.WrapOperation(err, "update saga instance in MongoDB")
	}
	if res.MatchedCount == 0 {
		// No document matched the {id, version} filter: either the instance is
		// gone, or its version moved on. Disambiguate so the caller sees
		// ErrInstanceNotFound vs ErrVersionConflict.
		return cas.ClassifyMiss(ctx, s.collection, collectionFieldId, inst.ID, saga.ErrInstanceNotFound, saga.ErrVersionConflict)
	}

	inst.Version = newVersion
	return nil
}

// FetchRecoverable returns up to limit non-terminal instances that are
// mid-compensation or past their deadline, restricted to definition when it is
// non-empty. A non-positive limit means no cap.
func (s *Store) FetchRecoverable(ctx context.Context, definition string, now time.Time, limit int) ([]*saga.Instance, error) {
	filter := bson.M{
		collectionFieldStatus: bson.M{opIn: bson.A{string(saga.StatusRunning), string(saga.StatusCompensating)}},
		"$and": bson.A{
			bson.M{"$or": bson.A{
				bson.M{collectionFieldStatus: string(saga.StatusCompensating)},
				bson.M{"lease_owner": bson.M{"$exists": true, "$ne": ""}},
				bson.M{collectionFieldDeadline: bson.M{"$gt": int64(0), "$lte": now.Unix()}},
			}},
			bson.M{"$or": bson.A{
				bson.M{collectionFieldLeaseUntil: bson.M{"$exists": false}},
				bson.M{collectionFieldLeaseUntil: bson.M{"$lte": now.UnixNano()}},
			}},
		},
	}
	if definition != "" {
		filter[collectionFieldDefinition] = definition
	}
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

// terminalStatuses lists the terminal instance statuses as BSON values.
func terminalStatuses() bson.A {
	return bson.A{string(saga.StatusCompleted), string(saga.StatusCompensated), string(saga.StatusFailed)}
}

// deleteTerminalBatch bounds the ids selected and deleted per round trip.
const deleteTerminalBatch = 500

// DeleteTerminal removes up to limit terminal instances last updated before
// olderThan, restricted to definition when it is non-empty, and returns how
// many it removed. A non-positive limit means no cap. updated_at is stored as
// Unix seconds, so olderThan is compared at that granularity.
func (s *Store) DeleteTerminal(ctx context.Context, definition string, olderThan time.Time, limit int) (int, error) {
	filter := bson.M{
		collectionFieldStatus:    bson.M{opIn: terminalStatuses()},
		collectionFieldUpdatedAt: bson.M{"$lt": olderThan.Unix()},
	}
	if definition != "" {
		filter[collectionFieldDefinition] = definition
	}

	deleted := 0
	for limit <= 0 || deleted < limit {
		batch := deleteTerminalBatch
		if limit > 0 {
			batch = min(batch, limit-deleted)
		}
		cursor, err := s.collection.Find(ctx, filter,
			mongoOptions.Find().SetProjection(bson.M{collectionFieldId: 1, collectionFieldVersion: 1}).SetLimit(int64(batch)))
		if err != nil {
			return deleted, coreerrs.WrapOperation(err, "find terminal saga instances in MongoDB")
		}
		var docs []struct {
			ID      string `bson:"_id"`
			Version int64  `bson:"version"`
		}
		err = cursor.All(ctx, &docs)
		if err != nil {
			return deleted, coreerrs.WrapOperation(err, "decode terminal saga instances from MongoDB")
		}
		if len(docs) == 0 {
			break
		}
		// Every eligibility predicate is rechecked with the observed version, so a
		// document replaced since the find (same _id, new record) is kept.
		candidates := make(bson.A, len(docs))
		for i, d := range docs {
			candidates[i] = bson.M{collectionFieldId: d.ID, collectionFieldVersion: d.Version}
		}
		deleteFilter := bson.M{}
		maps.Copy(deleteFilter, filter)
		deleteFilter["$or"] = candidates
		res, err := s.collection.DeleteMany(ctx, deleteFilter)
		if err != nil {
			return deleted, coreerrs.WrapOperation(err, "delete terminal saga instances from MongoDB")
		}
		deleted += int(res.DeletedCount)
		if len(docs) < batch || res.DeletedCount == 0 {
			break
		}
	}
	return deleted, nil
}

// Delete removes an instance. Deleting a missing instance is not an error.
func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.collection.DeleteOne(ctx, bson.D{{Key: collectionFieldId, Value: id}}); err != nil {
		return coreerrs.WrapOperation(err, "delete saga instance from MongoDB")
	}
	return nil
}

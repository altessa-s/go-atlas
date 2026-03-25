// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

import (
	"context"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/outbox"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoDB field names for the outbox event collection.
const (
	collectionFieldId            = "_id"             // Primary key field.
	collectionFieldPublishedAt   = "published_at"    // Publication timestamp field.
	collectionFieldStatus        = "status"          // Event status field.
	collectionFieldLockedOn      = "locked_on"       // Lock timestamp field.
	collectionFieldCreatedAt     = "created_at"      // Creation timestamp field.
	collectionFieldLastAttemptOn = "last_attempt_on" // Last attempt timestamp field.
	collectionFieldLastAttempts  = "attempts"        // Attempt count field.
	collectionFieldExpiresAt     = "expires_at"      // Expiration timestamp field.
)

// Store implements outbox.Store interface using MongoDB as the backend.
type Store struct {
	collection     *mongo.Collection
	collectionName string
	ctx            context.Context
	indexTimeout   time.Duration
}

// New creates a new MongoDB Store for outbox events.
// Creates required indexes automatically. Returns error if index creation fails.
//
// Example:
//
//	store, err := outboxstore.New(db, outboxstore.WithCollectionName("events"))
func New(db *mongo.Database, opt ...Option) (*Store, error) {
	opts := newOptions(opt...)
	s := &Store{
		collectionName: opts.collectionName,
		ctx:            opts.ctx,
		indexTimeout:   opts.indexTimeout,
	}
	s.ctx = corecontext.OrBackground(s.ctx)
	s.collection = db.Collection(s.collectionName)

	if err := s.createIndexes(); err != nil {
		return nil, coreerrs.WrapOperationWithContext(err, "create MongoDB indexes for outbox store", "collection '"+s.collectionName+"'")
	}

	return s, nil
}

// NewWithCollectionOptions creates a Store using an existing mongo.Collection, with Store options applied.
// It is useful when you already have a collection handle but still want to configure context/timeouts.
//
// Notes:
// - WithCollectionName has no effect (the provided collection is always used).
// - WithContext and WithIndexCreateTimeout affect index creation and other internal operations.
func NewWithCollectionOptions(col *mongo.Collection, opt ...Option) (*Store, error) {
	opts := newOptions(opt...)
	s := &Store{
		collection:     col,
		collectionName: col.Name(),
		ctx:            opts.ctx,
		indexTimeout:   opts.indexTimeout,
	}
	s.ctx = corecontext.OrBackground(s.ctx)

	if err := s.createIndexes(); err != nil {
		return nil, coreerrs.WrapOperationWithContext(err, "create MongoDB indexes for outbox store", "provided collection '"+s.collectionName+"'")
	}

	return s, nil
}

// UnlockStuckEvents resets in-progress events locked before since to pending status.
func (s *Store) UnlockStuckEvents(ctx context.Context, since time.Time) error {
	filter := bson.M{
		collectionFieldLockedOn: bson.M{"$lt": since.Unix()}, // Locked before the 'since' time
		collectionFieldStatus:   outbox.StatusInProgress,     // Only consider events currently in progress
	}
	update := bson.M{
		"$set": bson.M{
			collectionFieldStatus:   outbox.StatusPending, // Reset status to Pending
			collectionFieldLockedOn: 0,                    // Clear the lock time
		},
	}
	_, err := s.collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return coreerrs.WrapOperation(err, "unlock stuck events in MongoDB")
	}
	return nil
}

// DeleteProcessedEvents removes processed events (sent, skipped, or expired) older than since.
func (s *Store) DeleteProcessedEvents(ctx context.Context, since time.Time) error {
	filter := bson.M{
		collectionFieldPublishedAt: bson.M{"$lt": since.Unix()},
		collectionFieldStatus: bson.M{"$in": []outbox.Status{
			outbox.StatusSent, outbox.StatusSkipped, outbox.StatusExpired,
		}},
	}
	_, err := s.collection.DeleteMany(ctx, filter)
	if err != nil {
		return coreerrs.WrapOperation(err, "delete processed events from MongoDB")
	}
	return nil
}

// UpdateEvents performs a bulk update of event states in MongoDB.
// Returns nil if no events are provided.
func (s *Store) UpdateEvents(ctx context.Context, events ...outbox.Event) error {
	if len(events) == 0 {
		return nil
	}
	var writeModels = make([]mongo.WriteModel, 0, len(events))

	for i := range len(events) {
		ev := events[i]

		// bson.D (ordered slice) is more cache-friendly than bson.M (map) for
		// fixed-schema updates and avoids two map allocations per event.
		updateDoc := bson.D{{Key: "$set", Value: bson.D{
			{Key: collectionFieldStatus, Value: string(ev.Status)},
			{Key: collectionFieldLastAttemptOn, Value: ev.LastAttemptOn.Unix()},
			{Key: collectionFieldLockedOn, Value: ev.LockedOn.Unix()},
			{Key: collectionFieldLastAttempts, Value: ev.Attempts},
			{Key: collectionFieldPublishedAt, Value: ev.PublishedAt.Unix()},
		}}}

		model := mongo.NewUpdateOneModel().
			SetFilter(bson.D{{Key: collectionFieldId, Value: ev.Id}}).
			SetUpdate(updateDoc)
		writeModels = append(writeModels, model)
	}

	_, err := s.collection.BulkWrite(ctx, writeModels)
	if err != nil {
		return coreerrs.Wrap(err, "MongoDB BulkWrite failed for UpdateEvents")
	}
	return nil
}

// SaveEvents performs a bulk insert of new events into MongoDB.
// Returns nil if no events are provided.
func (s *Store) SaveEvents(ctx context.Context, events ...outbox.Event) error {
	if len(events) == 0 {
		return nil
	}
	var insertModels = make([]mongo.WriteModel, 0, len(events))

	for _, ev := range events {
		// Convert public outbox.Event to an internal MongoDB event entity.
		// Note: BSON field "event" is kept for database compatibility,
		// while the public API uses Event.Payload.
		doc := event{
			Id:        ev.Id,
			Status:    string(ev.Status),
			Event:     ev.Payload,
			CreatedAt: ev.CreatedAt.Unix(),
			Topic:     ev.Key,
			Attempts:  ev.Attempts,
			LastError: ev.LastError,
			// PublishedAt, LastAttemptOn, LockedOn are initially zero/default for new events
		}
		if !ev.ExpiresAt.IsZero() {
			doc.ExpiresAt = ev.ExpiresAt.Unix()
		}
		insertModels = append(insertModels, mongo.NewInsertOneModel().SetDocument(doc))
	}

	_, err := s.collection.BulkWrite(ctx, insertModels)
	if err != nil {
		return coreerrs.Wrap(err, "MongoDB BulkWrite failed for SaveEvents")
	}
	return nil
}

// FetchUnprocessedEvents retrieves pending or failed events and locks them for processing.
// Events are sorted by creation time; failed events must have last attempt before lastAttemptBefore.
func (s *Store) FetchUnprocessedEvents(ctx context.Context, batchSize uint32, lastAttemptBefore time.Time) ([]outbox.Event, error) {
	nowUnix := time.Now().UTC().Unix()
	// Exclude events whose ExpiresAt has passed.
	// The expires_at field uses omitempty, so documents without expiration have no field at all.
	notExpiredFilter := bson.M{"$or": bson.A{
		bson.M{collectionFieldExpiresAt: bson.M{"$exists": false}},
		bson.M{collectionFieldExpiresAt: bson.M{"$gt": nowUnix}},
	}}

	filter := bson.M{
		"$and": bson.A{
			bson.M{"$or": bson.A{
				bson.M{collectionFieldStatus: outbox.StatusPending},
				bson.M{
					collectionFieldStatus:        outbox.StatusFailed,
					collectionFieldLastAttemptOn: bson.M{"$lt": lastAttemptBefore.Unix()},
				},
			}},
			notExpiredFilter,
		},
	}

	var mongoEvents []event // Internal representation for MongoDB documents
	currentTime := time.Now().UTC()

	// Use a transaction to ensure Find + UpdateMany are atomic.
	// Without a transaction, another process could fetch the same pending events
	// between the Find and UpdateMany, leading to duplicate processing.
	sess, err := s.collection.Database().Client().StartSession()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "start session for FetchUnprocessedEvents")
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(sessCtx context.Context) (any, error) { //nolint:contextcheck
		// Reset on retry — WithTransaction may re-execute the callback on transient errors.
		mongoEvents = nil

		cursor, txErr := s.collection.Find(sessCtx, filter,
			mongoOptions.Find().SetSort(bson.M{collectionFieldCreatedAt: 1}).SetLimit(int64(batchSize)))
		if txErr != nil {
			return nil, coreerrs.Wrap(txErr, "MongoDB Find failed in FetchUnprocessedEvents")
		}
		defer func() { _ = cursor.Close(sessCtx) }()

		if txErr = cursor.All(sessCtx, &mongoEvents); txErr != nil {
			return nil, coreerrs.Wrap(txErr, "MongoDB cursor.All failed in FetchUnprocessedEvents")
		}

		if len(mongoEvents) == 0 {
			return 0, nil // No events to process
		}

		// Collect IDs of fetched events to update their status.
		idsToLock := slices.Collect(coreslices.Map(mongoEvents, func(e event) string { return e.Id }))

		// Update the status to InProgress and set LockedOn for the fetched events.
		update := bson.M{"$set": bson.M{
			collectionFieldStatus:   outbox.StatusInProgress,
			collectionFieldLockedOn: currentTime.Unix(),
		}}
		_, txErr = s.collection.UpdateMany(sessCtx, bson.M{collectionFieldId: bson.M{"$in": idsToLock}}, update)
		if txErr != nil {
			return nil, coreerrs.Wrap(txErr, "MongoDB UpdateMany (to lock events) failed in FetchUnprocessedEvents")
		}
		return 0, nil
	})

	if err != nil {
		return nil, err
	}
	if len(mongoEvents) == 0 {
		return []outbox.Event{}, nil // Return empty slice if no events fetched
	}

	// Convert MongoDB event entities to public outbox.Event type.
	// The LockedOn time is set to the time when they were locked in this operation.
	// Note: BSON field "event" maps to Event.Payload in the public API.
	return slices.Collect(coreslices.Map(mongoEvents, func(ev event) outbox.Event {
		e := outbox.Event{
			Id:            ev.Id,
			Key:           ev.Topic,
			Payload:       ev.Event,
			Status:        outbox.StatusInProgress,
			LastError:     ev.LastError,
			Attempts:      ev.Attempts,
			CreatedAt:     time.Unix(ev.CreatedAt, 0),
			PublishedAt:   time.Unix(ev.PublishedAt, 0),
			LastAttemptOn: time.Unix(ev.LastAttemptOn, 0),
			LockedOn:      currentTime, // Reflect the time they were locked in this fetch operation
		}
		if ev.ExpiresAt > 0 {
			e.ExpiresAt = time.Unix(ev.ExpiresAt, 0)
		}
		return e
	})), nil
}

// ExpireEvents marks pending or failed events whose ExpiresAt has passed as expired.
func (s *Store) ExpireEvents(ctx context.Context, now time.Time) (int64, error) {
	filter := bson.M{
		collectionFieldStatus:    bson.M{"$in": []outbox.Status{outbox.StatusPending, outbox.StatusFailed}},
		collectionFieldExpiresAt: bson.M{"$gt": int64(0), "$lte": now.Unix()},
	}
	update := bson.M{"$set": bson.M{
		collectionFieldStatus:      outbox.StatusExpired,
		collectionFieldPublishedAt: now.Unix(),
		collectionFieldLockedOn:    0,
	}}
	result, err := s.collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "expire events in MongoDB")
	}
	return result.ModifiedCount, nil
}

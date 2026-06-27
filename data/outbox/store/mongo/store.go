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

// serverNow is the MongoDB aggregation variable that yields the current datetime
// on the server (the primary at apply time). Using it instead of a client-supplied
// timestamp makes lock/retry/expiry decisions clock-skew safe: every replica sees
// the same single clock, so skewed wall clocks cannot disagree on whether an event
// is due, stuck, or expired. In a transaction $$NOW is the transaction start time,
// so the fetch filter and the lock update observe a consistent instant.
const serverNow = "$$NOW"

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

// UnlockStuckEvents resets in-progress events locked for longer than lockExpiry to
// pending. The lock age is evaluated server-side ($$NOW - lockExpiry) so a sweeper
// running on a skewed instance cannot prematurely unlock an event another instance
// is still publishing, nor leave a genuinely dead lock stuck.
func (s *Store) UnlockStuckEvents(ctx context.Context, lockExpiry time.Duration) error {
	filter := bson.M{
		collectionFieldStatus: outbox.StatusInProgress, // Only events currently in progress.
		"$expr": bson.M{"$lt": bson.A{
			"$" + collectionFieldLockedOn,
			bson.M{"$subtract": bson.A{serverNow, lockExpiry.Milliseconds()}},
		}},
	}
	// Aggregation-pipeline update so $$NOW-derived semantics stay server-side; the
	// lock is cleared (null) to mark the event unlocked.
	update := mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		collectionFieldStatus:   outbox.StatusPending,
		collectionFieldLockedOn: nil,
	}}}}
	if _, err := s.collection.UpdateMany(ctx, filter, update); err != nil {
		return coreerrs.WrapOperation(err, "unlock stuck events in MongoDB")
	}
	return nil
}

// DeleteProcessedEvents removes processed events (sent, skipped, or expired) whose
// publication is older than olderThan, evaluated server-side ($$NOW - olderThan).
func (s *Store) DeleteProcessedEvents(ctx context.Context, olderThan time.Duration) error {
	filter := bson.M{
		collectionFieldStatus: bson.M{"$in": []outbox.Status{
			outbox.StatusSent, outbox.StatusSkipped, outbox.StatusExpired,
		}},
		"$expr": bson.M{"$lt": bson.A{
			"$" + collectionFieldPublishedAt,
			bson.M{"$subtract": bson.A{serverNow, olderThan.Milliseconds()}},
		}},
	}
	if _, err := s.collection.DeleteMany(ctx, filter); err != nil {
		return coreerrs.WrapOperation(err, "delete processed events from MongoDB")
	}
	return nil
}

// UpdateEvents performs a bulk update of event states in MongoDB.
// Returns nil if no events are provided.
//
// last_attempt_on is stamped with the server clock ($$NOW) rather than a
// client timestamp, so the retry gate in FetchUnprocessedEvents (which also
// compares against $$NOW) stays clock-skew safe regardless of which instance
// dispatched the event. The lock is cleared so the event leaves the in-progress set.
func (s *Store) UpdateEvents(ctx context.Context, events ...outbox.Event) error {
	if len(events) == 0 {
		return nil
	}
	var writeModels = make([]mongo.WriteModel, 0, len(events))

	for i := range len(events) {
		ev := events[i]

		set := bson.M{
			collectionFieldStatus:        string(ev.Status),
			collectionFieldLastAttempts:  ev.Attempts,
			collectionFieldLastAttemptOn: serverNow,
			collectionFieldLockedOn:      nullableDate(ev.LockedOn),
			collectionFieldPublishedAt:   nullableDate(ev.PublishedAt),
		}
		// Aggregation-pipeline update is required to reference $$NOW.
		model := mongo.NewUpdateOneModel().
			SetFilter(bson.D{{Key: collectionFieldId, Value: ev.Id}}).
			SetUpdate(mongo.Pipeline{bson.D{{Key: "$set", Value: set}}})
		writeModels = append(writeModels, model)
	}

	if _, err := s.collection.BulkWrite(ctx, writeModels); err != nil {
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
			CreatedAt: ev.CreatedAt.UTC(),
			Topic:     ev.Key,
			Attempts:  ev.Attempts,
			LastError: ev.LastError,
			ExpiresAt: nullableDate(ev.ExpiresAt),
			// PublishedAt, LastAttemptOn, LockedOn are nil for new events.
		}
		insertModels = append(insertModels, mongo.NewInsertOneModel().SetDocument(doc))
	}

	if _, err := s.collection.BulkWrite(ctx, insertModels); err != nil {
		return coreerrs.Wrap(err, "MongoDB BulkWrite failed for SaveEvents")
	}
	return nil
}

// FetchUnprocessedEvents retrieves pending events, and failed events whose last
// attempt is older than retryAfter, then locks them for processing.
//
// Both the retry deadline ($$NOW - retryAfter) and the not-yet-expired predicate
// ($expires_at > $$NOW) are evaluated against the MongoDB server clock, and the
// lock timestamp is written as $$NOW — never a client time — so concurrent
// instances with skewed wall clocks agree on eligibility and lock freshness.
func (s *Store) FetchUnprocessedEvents(ctx context.Context, batchSize uint32, retryAfter time.Duration) ([]outbox.Event, error) {
	// Exclude events whose ExpiresAt has passed. Documents without expiration omit
	// the field entirely (pointer + omitempty), so $exists:false admits them.
	notExpiredFilter := bson.M{"$or": bson.A{
		bson.M{collectionFieldExpiresAt: bson.M{"$exists": false}},
		bson.M{"$expr": bson.M{"$gt": bson.A{"$" + collectionFieldExpiresAt, serverNow}}},
	}}

	// Pending events are always eligible; failed events only once their last
	// attempt is older than retryAfter relative to the server clock.
	readyFilter := bson.M{"$or": bson.A{
		bson.M{collectionFieldStatus: outbox.StatusPending},
		bson.M{
			collectionFieldStatus: outbox.StatusFailed,
			"$expr": bson.M{"$lt": bson.A{
				"$" + collectionFieldLastAttemptOn,
				bson.M{"$subtract": bson.A{serverNow, retryAfter.Milliseconds()}},
			}},
		},
	}}

	filter := bson.M{"$and": bson.A{readyFilter, notExpiredFilter}}

	var (
		mongoEvents []event
		// lockedAt is informational only: the authoritative lock timestamp is the
		// server's $$NOW written below. The returned LockedOn is cleared by the
		// dispatcher before the event is used, so an approximate client value is fine.
		lockedAt time.Time
	)

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
		lockedAt = time.Now().UTC()

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

		// Collect IDs of fetched events to lock them.
		idsToLock := slices.Collect(coreslices.Map(mongoEvents, func(e event) string { return e.Id }))

		// Lock via aggregation-pipeline update so locked_on carries the server clock.
		update := mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{
			collectionFieldStatus:   outbox.StatusInProgress,
			collectionFieldLockedOn: serverNow,
		}}}}
		if _, txErr = s.collection.UpdateMany(sessCtx, bson.M{collectionFieldId: bson.M{"$in": idsToLock}}, update); txErr != nil {
			return nil, coreerrs.Wrap(txErr, "MongoDB UpdateMany (to lock events) failed in FetchUnprocessedEvents")
		}
		return 0, nil
	})

	if err != nil {
		return nil, err
	}
	if len(mongoEvents) == 0 {
		return []outbox.Event{}, nil
	}

	// Convert MongoDB event entities to public outbox.Event type.
	// Note: BSON field "event" maps to Event.Payload in the public API.
	return slices.Collect(coreslices.Map(mongoEvents, func(ev event) outbox.Event {
		return outbox.Event{
			Id:            ev.Id,
			Key:           ev.Topic,
			Payload:       ev.Event,
			Status:        outbox.StatusInProgress,
			LastError:     ev.LastError,
			Attempts:      ev.Attempts,
			CreatedAt:     ev.CreatedAt,
			PublishedAt:   dateValue(ev.PublishedAt),
			LastAttemptOn: dateValue(ev.LastAttemptOn),
			ExpiresAt:     dateValue(ev.ExpiresAt),
			LockedOn:      lockedAt,
		}
	})), nil
}

// ExpireEvents marks pending or failed events whose ExpiresAt has passed as expired.
// The deadline is evaluated against the server clock ($expires_at <= $$NOW), and
// published_at is stamped with $$NOW so subsequent cleanup is clock-skew safe too.
func (s *Store) ExpireEvents(ctx context.Context) (int64, error) {
	filter := bson.M{
		collectionFieldStatus:    bson.M{"$in": []outbox.Status{outbox.StatusPending, outbox.StatusFailed}},
		collectionFieldExpiresAt: bson.M{"$exists": true},
		"$expr":                  bson.M{"$lte": bson.A{"$" + collectionFieldExpiresAt, serverNow}},
	}
	update := mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		collectionFieldStatus:      outbox.StatusExpired,
		collectionFieldPublishedAt: serverNow,
		collectionFieldLockedOn:    nil,
	}}}}
	result, err := s.collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "expire events in MongoDB")
	}
	return result.ModifiedCount, nil
}

// nullableDate returns a UTC *time.Time for a non-zero instant, or nil so the field
// is omitted/null in MongoDB. A nil pointer keeps "unset" distinct from a real date.
func nullableDate(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// dateValue dereferences a stored *time.Time, returning the zero time when absent.
func dateValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

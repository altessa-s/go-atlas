// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/audit"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoDB field names and server error codes.
const (
	fieldID               = "_id"
	fieldType             = "type"
	fieldAction           = "action"
	fieldTimestamp        = "timestamp"
	fieldActorID          = "actor.id"
	fieldActorType        = "actor.type"
	fieldResourceType     = "resource.type"
	fieldResourceID       = "resource.id"
	fieldResultStatus     = "result.status"
	fieldContextRequestID = "context.request_id"
	fieldContextTraceID   = "context.trace_id"

	duplicateKeyErrorCode = 11000
)

var _ audit.Storage = (*Storage)(nil)

// Storage implements audit.Storage using MongoDB.
type Storage struct {
	collection *mongo.Collection
	opts       *options
}

// New creates a new MongoDB audit Storage and ensures indexes exist.
// Index creation runs on ctx. Sub-second positive TTLs and TTLs overflowing
// MongoDB's 32-bit expireAfterSeconds option fail with [coreerrs.ErrInvalidArgument].
func New(ctx context.Context, db *mongo.Database, opts ...Option) (*Storage, error) {
	o := newOptions(opts...)
	if err := validateTTL(o.ttl); err != nil {
		return nil, err
	}
	if db == nil {
		return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "mongo database is required")
	}

	s := &Storage{
		collection: db.Collection(o.collectionName),
		opts:       o,
	}

	if err := s.createIndexes(ctx); err != nil {
		return nil, coreerrs.WrapOperation(err, "create audit indexes")
	}

	return s, nil
}

func validateTTL(ttl time.Duration) error {
	if ttl > 0 && (ttl < MinTTL || ttl > MaxTTL) {
		return coreerrs.Wrapf(coreerrs.ErrInvalidArgument,
			"audit mongo ttl %s must be 0 or between %s and %s", ttl, MinTTL, MaxTTL)
	}
	return nil
}

// Store inserts a single audit event into MongoDB. Replaying an event whose ID
// already exists in the collection succeeds without duplicating it.
func (s *Storage) Store(ctx context.Context, event *audit.Event) error {
	if event == nil {
		return coreerrs.Wrap(coreerrs.ErrInvalidArgument, "nil audit event")
	}
	_, err := s.collection.InsertOne(ctx, toModel(event))
	return s.handleInsertError(ctx, []*audit.Event{event}, err)
}

// StoreBatch inserts multiple audit events into MongoDB in a single unordered
// InsertMany call. Events whose ID already exists in the collection (or earlier
// in the same batch) are verified by reading back their _id and acknowledged as
// replays; write-concern errors, non-duplicate write errors, and duplicate
// errors on another unique index propagate as safe count-only errors.
func (s *Storage) StoreBatch(ctx context.Context, events []*audit.Event) error {
	if len(events) == 0 {
		return nil
	}

	models := make([]any, len(events))
	for i, e := range events {
		if e == nil {
			return coreerrs.Wrap(coreerrs.ErrInvalidArgument, "nil audit event")
		}
		models[i] = toModel(e)
	}

	_, err := s.collection.InsertMany(ctx, models, mongoOptions.InsertMany().SetOrdered(false))
	return s.handleInsertError(ctx, events, err)
}

func (s *Storage) handleInsertError(ctx context.Context, events []*audit.Event, err error) error {
	if err == nil {
		return nil
	}

	wce, writeErrs, ok := extractWriteException(err)
	if !ok {
		return coreerrs.WrapOperation(err, "store audit events")
	}
	if wce != nil {
		return fmt.Errorf("store audit events: write concern error (code %d, write_errors=%d, batch=%d)",
			wce.Code, len(writeErrs), len(events))
	}
	if len(writeErrs) == 0 {
		return fmt.Errorf("store audit events: bulk write failed with 0 write errors (batch=%d)", len(events))
	}

	seen := make(map[string]struct{}, len(writeErrs))
	uniqueIDs := make([]string, 0, len(writeErrs))
	var dupCount, otherCount int
	for _, we := range writeErrs {
		if we.Code != duplicateKeyErrorCode || we.Index < 0 || we.Index >= len(events) {
			otherCount++
			continue
		}
		dupCount++
		id := events[we.Index].ID
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			uniqueIDs = append(uniqueIDs, id)
		}
	}
	if otherCount > 0 {
		return fmt.Errorf("store audit events: %d of %d writes failed (%d duplicate_key, %d other)",
			len(writeErrs), len(events), dupCount, otherCount)
	}

	existing, findErr := s.existingIDs(ctx, uniqueIDs)
	if findErr != nil {
		return coreerrs.WrapOperation(findErr, "verify duplicate audit events")
	}

	var verified, unverified int
	for _, we := range writeErrs {
		if _, ok := existing[events[we.Index].ID]; ok {
			verified++
		} else {
			unverified++
		}
	}
	if unverified > 0 {
		return fmt.Errorf("store audit events: %d of %d writes rejected on unique constraint (%d verified_replays, %d unverified)",
			len(writeErrs), len(events), verified, unverified)
	}
	return nil
}

func extractWriteException(err error) (*mongo.WriteConcernError, []mongo.WriteError, bool) {
	if bwe, ok := errors.AsType[mongo.BulkWriteException](err); ok {
		errs := make([]mongo.WriteError, len(bwe.WriteErrors))
		for i, item := range bwe.WriteErrors {
			errs[i] = item.WriteError
		}
		return bwe.WriteConcernError, errs, true
	}
	if we, ok := errors.AsType[mongo.WriteException](err); ok {
		return we.WriteConcernError, we.WriteErrors, true
	}
	return nil, nil, false
}

func (s *Storage) existingIDs(ctx context.Context, ids []string) (map[string]struct{}, error) {
	filter := bson.D{{Key: fieldID, Value: bson.D{{Key: "$in", Value: ids}}}}
	opts := mongoOptions.Find().
		SetProjection(bson.D{{Key: fieldID, Value: 1}}).
		SetCollation(simpleCollation())

	cursor, err := s.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	found := make(map[string]struct{}, len(ids))
	for cursor.Next(ctx) {
		var doc struct {
			ID string `bson:"_id"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		found[doc.ID] = struct{}{}
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return found, nil
}

// Query returns an iterator over events matching the query, ordered by
// timestamp then ID, newest first unless ascending order is requested, and
// starting after query.Cursor.
func (s *Storage) Query(ctx context.Context, query *audit.Query) iter.Seq2[*audit.Event, error] {
	if query == nil {
		query = &audit.Query{}
	}
	return func(yield func(*audit.Event, error) bool) {
		filter := buildFilter(query)

		sortOrder := -1 // desc by default
		if query.SortOrder == audit.SortOrderAsc {
			sortOrder = 1
		}

		if query.Cursor != nil {
			filter = bson.D{{Key: "$and", Value: bson.A{filter, cursorFilter(query.Cursor, sortOrder)}}}
		}

		// (timestamp, _id) is the total order keyset pagination relies on.
		// The simple collation compares IDs bytewise like the cursor
		// predicate does, whatever collation the collection defaults to.
		opts := mongoOptions.Find().
			SetSort(bson.D{{Key: fieldTimestamp, Value: sortOrder}, {Key: fieldID, Value: sortOrder}}).
			SetCollation(simpleCollation())

		if query.Limit > 0 {
			opts.SetLimit(int64(query.Limit))
		}

		cursor, err := s.collection.Find(ctx, filter, opts)
		if err != nil {
			yield(nil, coreerrs.WrapOperation(err, "query audit events"))
			return
		}
		defer func() { _ = cursor.Close(ctx) }()

		for cursor.Next(ctx) {
			var m eventModel
			if err := cursor.Decode(&m); err != nil {
				yield(nil, coreerrs.WrapOperation(err, "decode audit event"))
				return
			}
			if !yield(fromModel(&m), nil) {
				return
			}
		}

		if err := cursor.Err(); err != nil {
			yield(nil, coreerrs.WrapOperation(err, "iterate audit events"))
		}
	}
}

// Count returns the number of events matching the query filter. The page
// position, size and order are ignored.
func (s *Storage) Count(ctx context.Context, query *audit.Query) (int64, error) {
	if query == nil {
		query = &audit.Query{}
	}
	filter := buildFilter(query)
	count, err := s.collection.CountDocuments(ctx, filter, mongoOptions.Count().SetCollation(simpleCollation()))
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "count audit events")
	}
	return count, nil
}

// Close is a no-op; the caller manages the underlying [mongo.Database] lifecycle.
func (s *Storage) Close(_ context.Context) error {
	return nil
}

// simpleCollation compares strings bytewise. Queries use it so that ordering
// and equality do not depend on the collection's default collation: a
// case-insensitive one would make distinct IDs compare equal and keyset
// paging skip events.
func simpleCollation() *mongoOptions.Collation {
	return &mongoOptions.Collation{Locale: "simple"}
}

// cursorFilter selects the events strictly after c in the given order (1
// ascending, -1 descending). BSON dates hold milliseconds, matching the
// cursor's precision.
func cursorFilter(c *audit.Cursor, order int) bson.D {
	op := "$lt"
	if order > 0 {
		op = "$gt"
	}
	ts := time.UnixMilli(c.Timestamp.UnixMilli()).UTC()
	return bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: fieldTimestamp, Value: bson.D{{Key: op, Value: ts}}}},
		bson.D{
			{Key: fieldTimestamp, Value: ts},
			{Key: fieldID, Value: bson.D{{Key: op, Value: c.ID}}},
		},
	}}}
}

func buildFilter(q *audit.Query) bson.D {
	filter := bson.D{}

	if q.StartTime != nil || q.EndTime != nil {
		ts := bson.D{}
		ts = slices.AppendIfFunc(ts, q.StartTime != nil, func() []bson.E {
			return []bson.E{{Key: "$gte", Value: *q.StartTime}}
		})
		ts = slices.AppendIfFunc(ts, q.EndTime != nil, func() []bson.E {
			return []bson.E{{Key: "$lte", Value: *q.EndTime}}
		})
		filter = append(filter, bson.E{Key: fieldTimestamp, Value: ts})
	}

	filter = slices.AppendIf(filter, q.ActorID != "", bson.E{Key: fieldActorID, Value: q.ActorID})
	filter = slices.AppendIf(filter, q.ActorType != "", bson.E{Key: fieldActorType, Value: q.ActorType})
	filter = slices.AppendIf(filter, q.ResourceType != "", bson.E{Key: fieldResourceType, Value: q.ResourceType})
	filter = slices.AppendIf(filter, q.ResourceID != "", bson.E{Key: fieldResourceID, Value: q.ResourceID})
	filter = slices.AppendIf(filter, q.EventType != "", bson.E{Key: fieldType, Value: string(q.EventType)})
	filter = slices.AppendIf(filter, q.Action != "", bson.E{Key: fieldAction, Value: string(q.Action)})
	filter = slices.AppendIf(filter, q.Status != "", bson.E{Key: fieldResultStatus, Value: string(q.Status)})
	filter = slices.AppendIf(filter, q.RequestID != "", bson.E{Key: fieldContextRequestID, Value: q.RequestID})
	filter = slices.AppendIf(filter, q.TraceID != "", bson.E{Key: fieldContextTraceID, Value: q.TraceID})

	return filter
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/altessa-s/go-atlas/audit"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoDB field names, batching bounds, and server error codes.
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

	// maxInsertBatchDocs and maxInsertBatchBytes bound each InsertMany call so
	// the driver never splits one call across multiple OP_MSG wire batches
	// (well below MongoDB's 100,000-document maxWriteBatchSize, 48 MB
	// maxMessageSizeBytes, and 2 MiB CSFLE split threshold), preventing
	// mongo-driver v2.10.1 from overwriting an earlier batch's WriteConcernError.
	maxInsertBatchDocs  = 1000
	maxInsertBatchBytes = 2_000_000

	// Conservative BSON framing estimates used by approxEventBytes.
	bsonEventEnvelopeBytes = 512
	bsonChangesHeaderBytes = 64
	bsonErrorHeaderBytes   = 32
	bsonElemHeaderBytes    = 16
	bsonKeyNullBytes       = 4
	bsonLengthNullBytes    = 8
	bsonScalarBytes        = 16
	bsonUnknownValueBytes  = 256
)

var _ audit.Storage = (*Storage)(nil)

// Storage implements audit.Storage using MongoDB.
type Storage struct {
	collection *mongo.Collection
	verifyColl *mongo.Collection
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

	coll := db.Collection(o.collectionName)
	s := &Storage{
		collection: coll,
		verifyColl: newVerifyCollection(coll),
		opts:       o,
	}

	if err := s.createIndexes(ctx); err != nil {
		return nil, safeMongoError("create audit indexes", err)
	}

	return s, nil
}

func newVerifyCollection(coll *mongo.Collection) *mongo.Collection {
	return coll.Clone(mongoOptions.Collection().
		SetReadPreference(readpref.Primary()).
		SetReadConcern(readconcern.Majority()))
}

func (s *Storage) verifyCollection() *mongo.Collection {
	if s.verifyColl != nil {
		return s.verifyColl
	}
	return newVerifyCollection(s.collection)
}

func validateTTL(ttl time.Duration) error {
	if ttl > 0 && (ttl < MinTTL || ttl > MaxTTL) {
		return coreerrs.Wrapf(coreerrs.ErrInvalidArgument,
			"audit mongo ttl %s must be 0 or between %s and %s", ttl, MinTTL, MaxTTL)
	}
	return nil
}

// Store inserts a single audit event into MongoDB. A nil event fails with
// [coreerrs.ErrInvalidArgument].
//
// Replay detection is keyed by Event.ID (_id) only: when MongoDB reports code
// 11000 and a document with that _id is confirmed on the primary under majority
// read concern, Store succeeds without modifying the stored document. Callers
// using Storage directly without [audit.Auditor] must supply unique IDs.
// Unverified unique-index conflicts wrap [coreerrs.ErrConflict] and
// write-concern failures wrap [coreerrs.ErrUnavailable].
func (s *Storage) Store(ctx context.Context, event *audit.Event) error {
	if event == nil {
		return coreerrs.Wrap(coreerrs.ErrInvalidArgument, "nil audit event")
	}
	_, err := s.collection.InsertOne(ctx, toModel(event))
	if err == nil {
		return nil
	}
	wce, writeErrs, ok := extractWriteException(err)
	if !ok {
		return safeMongoError("store audit events", err)
	}
	return s.resolveWriteFailures(ctx, []*audit.Event{event}, wce, writeErrs)
}

// StoreBatch inserts multiple audit events into MongoDB using unordered
// InsertMany sub-batches bounded so each call fits in one wire batch. A nil
// element rejects the whole batch with [coreerrs.ErrInvalidArgument] before any
// write is sent.
//
// Replay detection is keyed by Event.ID (_id) only: duplicate-key (11000)
// errors are verified by reading _id from the primary with majority read
// concern and acknowledged only when every rejected ID exists. Any
// write-concern failure across any sub-batch fails the call with
// [coreerrs.ErrUnavailable], unverified unique-constraint failures wrap
// [coreerrs.ErrConflict], and non-duplicate write errors wrap
// [coreerrs.ErrFailedPrecondition], always with safe count-only descriptions.
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

	var (
		firstWCE     *mongo.WriteConcernError
		allWriteErrs []mongo.WriteError
	)

	for start := 0; start < len(events); {
		end := nextBatchEnd(events, start)
		_, err := s.collection.InsertMany(ctx, models[start:end], mongoOptions.InsertMany().SetOrdered(false))
		if err != nil {
			wce, batchWriteErrs, ok := extractWriteException(err)
			if !ok {
				return safeMongoError("store audit events", err)
			}
			if wce != nil && firstWCE == nil {
				firstWCE = wce
			}
			if wce == nil && len(batchWriteErrs) == 0 {
				return coreerrs.Wrapf(coreerrs.ErrFailedPrecondition,
					"store audit events: bulk write failed with 0 write errors (batch=%d)", len(events))
			}
			for _, we := range batchWriteErrs {
				if we.Index >= 0 {
					we.Index += start
				}
				allWriteErrs = append(allWriteErrs, we)
			}
		}
		start = end
	}

	if firstWCE == nil && len(allWriteErrs) == 0 {
		return nil
	}
	return s.resolveWriteFailures(ctx, events, firstWCE, allWriteErrs)
}

func nextBatchEnd(events []*audit.Event, start int) int {
	end := start
	var batchBytes int
	for end < len(events) && end-start < maxInsertBatchDocs {
		docBytes := approxEventBytes(events[end])
		if end > start && batchBytes+docBytes > maxInsertBatchBytes {
			break
		}
		batchBytes += docBytes
		end++
	}
	return end
}

func (s *Storage) resolveWriteFailures(
	ctx context.Context,
	events []*audit.Event,
	wce *mongo.WriteConcernError,
	writeErrs []mongo.WriteError,
) error {
	if wce != nil {
		return coreerrs.Wrapf(coreerrs.ErrUnavailable,
			"store audit events: write concern failed (code=%d, write_errors=%d, batch=%d)",
			wce.Code, len(writeErrs), len(events))
	}
	if len(writeErrs) == 0 {
		return coreerrs.Wrapf(coreerrs.ErrFailedPrecondition,
			"store audit events: bulk write failed with 0 write errors (batch=%d)", len(events))
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
		return coreerrs.Wrapf(coreerrs.ErrFailedPrecondition,
			"store audit events: %d of %d writes failed (%d duplicate_key, %d other)",
			len(writeErrs), len(events), dupCount, otherCount)
	}

	existing, findErr := s.existingIDs(ctx, uniqueIDs)
	if findErr != nil {
		return safeMongoError("verify duplicate audit events", findErr)
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
		return coreerrs.Wrapf(coreerrs.ErrConflict,
			"store audit events: %d of %d writes rejected on unique constraint (%d verified_replays, %d unverified)",
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
	coll := s.verifyCollection()
	found := make(map[string]struct{}, len(ids))
	opts := mongoOptions.Find().
		SetProjection(bson.D{{Key: fieldID, Value: 1}}).
		SetCollation(simpleCollation())

	for start := 0; start < len(ids); start += maxInsertBatchDocs {
		chunk := ids[start:min(start+maxInsertBatchDocs, len(ids))]
		filter := bson.D{{Key: fieldID, Value: bson.D{{Key: "$in", Value: chunk}}}}

		cursor, err := coll.Find(ctx, filter, opts)
		if err != nil {
			return nil, err
		}
		err = readCursorIDs(ctx, cursor, found)
		_ = cursor.Close(ctx)
		if err != nil {
			return nil, err
		}
	}
	return found, nil
}

func readCursorIDs(ctx context.Context, cursor *mongo.Cursor, found map[string]struct{}) error {
	for cursor.Next(ctx) {
		var doc struct {
			ID string `bson:"_id"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return err
		}
		found[doc.ID] = struct{}{}
	}
	return cursor.Err()
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
			yield(nil, safeMongoError("query audit events", err))
			return
		}
		defer func() { _ = cursor.Close(ctx) }()

		for cursor.Next(ctx) {
			var m eventModel
			if err := cursor.Decode(&m); err != nil {
				yield(nil, safeMongoError("decode audit event", err))
				return
			}
			if !yield(fromModel(&m), nil) {
				return
			}
		}

		if err := cursor.Err(); err != nil {
			yield(nil, safeMongoError("iterate audit events", err))
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
		return 0, safeMongoError("count audit events", err)
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
		ts = coreslices.AppendIfFunc(ts, q.StartTime != nil, func() []bson.E {
			return []bson.E{{Key: "$gte", Value: *q.StartTime}}
		})
		ts = coreslices.AppendIfFunc(ts, q.EndTime != nil, func() []bson.E {
			return []bson.E{{Key: "$lte", Value: *q.EndTime}}
		})
		filter = append(filter, bson.E{Key: fieldTimestamp, Value: ts})
	}

	filter = coreslices.AppendIf(filter, q.ActorID != "", bson.E{Key: fieldActorID, Value: q.ActorID})
	filter = coreslices.AppendIf(filter, q.ActorType != "", bson.E{Key: fieldActorType, Value: q.ActorType})
	filter = coreslices.AppendIf(filter, q.ResourceType != "", bson.E{Key: fieldResourceType, Value: q.ResourceType})
	filter = coreslices.AppendIf(filter, q.ResourceID != "", bson.E{Key: fieldResourceID, Value: q.ResourceID})
	filter = coreslices.AppendIf(filter, q.EventType != "", bson.E{Key: fieldType, Value: string(q.EventType)})
	filter = coreslices.AppendIf(filter, q.Action != "", bson.E{Key: fieldAction, Value: string(q.Action)})
	filter = coreslices.AppendIf(filter, q.Status != "", bson.E{Key: fieldResultStatus, Value: string(q.Status)})
	filter = coreslices.AppendIf(filter, q.RequestID != "", bson.E{Key: fieldContextRequestID, Value: q.RequestID})
	filter = coreslices.AppendIf(filter, q.TraceID != "", bson.E{Key: fieldContextTraceID, Value: q.TraceID})

	return filter
}

// safeMongoError summarizes a MongoDB driver or server failure without raw
// server messages (which can echo query filters, event IDs, or connection
// details). It preserves context cancellation/deadline sentinels by identity
// and classifies network and timeout errors under [coreerrs.ErrUnavailable].
func safeMongoError(op string, err error) error {
	if err == nil {
		return nil
	}
	var sentinels []error
	for _, s := range []error{context.Canceled, context.DeadlineExceeded} {
		if chainHolds(err, s) {
			sentinels = append(sentinels, s)
		}
	}
	_, isNetErr := errors.AsType[net.Error](err)
	if !slices.Contains(sentinels, context.Canceled) && (isNetErr || mongo.IsNetworkError(err) || mongo.IsTimeout(err)) {
		sentinels = append(sentinels, coreerrs.ErrUnavailable)
	}

	msg := fmt.Sprintf("failed to %s (%T)", op, err)
	if cmdErr, ok := errors.AsType[mongo.CommandError](err); ok && cmdErr.Code != 0 {
		msg = fmt.Sprintf("failed to %s (%T, code=%d)", op, err, cmdErr.Code)
	}
	return &mongoSafeError{msg: msg, sentinels: sentinels}
}

type mongoSafeError struct {
	msg       string
	sentinels []error
}

func (e *mongoSafeError) Error() string { return e.msg }

func (e *mongoSafeError) Is(target error) bool {
	return slices.Contains(e.sentinels, target)
}

func chainHolds(err, target error) bool {
	for err != nil {
		if err == target { //nolint:errorlint // identity check avoids invoking untrusted custom Is methods
			return true
		}
		switch u := err.(type) { //nolint:errorlint // manual chain walk by design
		case interface{ Unwrap() []error }:
			for _, inner := range u.Unwrap() {
				if chainHolds(inner, target) {
					return true
				}
			}
			return false
		case interface{ Unwrap() error }:
			err = u.Unwrap()
		default:
			return false
		}
	}
	return false
}

// approxEventBytes returns a conservative upper bound on the BSON byte size of e
// without requiring a BSON registry lookup on the fast path.
func approxEventBytes(e *audit.Event) int {
	n := bsonEventEnvelopeBytes +
		len(e.ID) + len(e.Type) + len(e.Action) +
		len(e.Actor.Type) + len(e.Actor.ID) + len(e.Actor.Name) +
		len(e.Actor.Email) + len(e.Actor.IP) + len(e.Actor.UserAgent) +
		len(e.Resource.Type) + len(e.Resource.ID) + len(e.Resource.Name) + len(e.Resource.Path) +
		len(e.Result.Status) + len(e.Result.Message) +
		len(e.Context.RequestID) + len(e.Context.TraceID) +
		len(e.Context.SpanID) + len(e.Context.CorrelationID) +
		len(e.Service.Name) + len(e.Service.Version) + len(e.Service.Instance) +
		approxStringSliceBytes(e.Actor.Roles) +
		approxMapBytes(e.Actor.Metadata) +
		approxMapBytes(e.Resource.Attributes) +
		approxMapBytes(e.Metadata)
	if e.Resource.Changes != nil {
		n += bsonChangesHeaderBytes +
			approxMapBytes(e.Resource.Changes.Before) +
			approxMapBytes(e.Resource.Changes.After) +
			approxStringSliceBytes(e.Resource.Changes.Fields)
	}
	if e.Result.Error != nil {
		n += bsonErrorHeaderBytes + len(e.Result.Error.Code) + len(e.Result.Error.Message)
	}
	return n
}

func approxStringSliceBytes(items []string) int {
	if len(items) == 0 {
		return 0
	}
	n := bsonElemHeaderBytes
	for _, s := range items {
		n += bsonElemHeaderBytes + len(s)
	}
	return n
}

func approxMapBytes(m map[string]any) int {
	if len(m) == 0 {
		return 0
	}
	n := bsonElemHeaderBytes
	for k, v := range m {
		n += len(k) + bsonKeyNullBytes + approxValueBytes(v)
	}
	return n
}

func approxValueBytes(v any) int {
	switch x := v.(type) {
	case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, time.Time:
		return bsonScalarBytes
	case string:
		return len(x) + bsonLengthNullBytes
	case json.Number:
		return len(x) + bsonElemHeaderBytes
	case []byte:
		return len(x) + bsonLengthNullBytes
	case []string:
		return approxStringSliceBytes(x)
	case map[string]any:
		return approxMapBytes(x)
	case bson.M:
		return approxMapBytes(x)
	case []any:
		n := bsonElemHeaderBytes
		for _, elem := range x {
			n += bsonElemHeaderBytes + approxValueBytes(elem)
		}
		return n
	default:
		if _, raw, err := bson.MarshalValue(v); err == nil {
			return len(raw) + bsonElemHeaderBytes
		}
		return bsonUnknownValueBytes
	}
}

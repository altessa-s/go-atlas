// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"time"
	"uuid"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
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
	collectionFieldLastError     = "error"           // Last dispatch error field.
	collectionFieldLockToken     = "lock_token"      // Fencing token field.
	collectionFieldNextAttemptAt = "next_attempt_at" // Retry-eligibility timestamp field.
	collectionFieldSeq           = "seq"             // Insertion-order tie-breaker field.
)

// serverNow is the MongoDB aggregation variable that yields the current datetime
// on the server (the primary at apply time). Using it instead of a client-supplied
// timestamp makes lock/retry/expiry decisions clock-skew safe: every replica sees
// the same single clock, so skewed wall clocks cannot disagree on whether an event
// is due, stuck, or expired. In a transaction $$NOW is the transaction start time,
// so the fetch filter and the lock update observe a consistent instant.
const serverNow = "$$NOW"

// Aggregation and query operators the store's pipelines reuse.
const (
	// opCond is the ternary aggregation operator: [condition, then, else].
	opCond = "$cond"

	opOr     = "$or"     // Logical disjunction.
	opEq     = "$eq"     // Equality comparison.
	opLte    = "$lte"    // Less-than-or-equal comparison.
	opLt     = "$lt"     // Less-than comparison.
	opExists = "$exists" // Field-presence test.
	opSum    = "$sum"    // Accumulator used by the stats aggregation.

	opSet      = "$set"      // Field assignment (update and pipeline stage).
	opIn       = "$in"       // Membership test.
	opExpr     = "$expr"     // Aggregation expression inside a query filter.
	opSubtract = "$subtract" // Arithmetic or date difference.
)

// Store implements outbox.Storage interface using MongoDB as the backend.
type Store struct {
	collection     *mongo.Collection
	collectionName string
	ctx            context.Context
	indexTimeout   time.Duration
	logger         *slog.Logger
}

// New creates a new MongoDB Store for outbox events.
// Creates required indexes automatically. Returns error if index creation fails.
//
// Example:
//
//	store, err := outboxmongo.New(db, outboxmongo.WithCollectionName("events"))
func New(db *mongo.Database, opt ...Option) (*Store, error) {
	opts := newOptions(opt...)
	s := &Store{
		collectionName: opts.collectionName,
		ctx:            opts.ctx,
		indexTimeout:   opts.indexTimeout,
		logger:         opts.logger,
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
		logger:         opts.logger,
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
//
// The lock token is cleared along with the lock, which is what makes reclaiming
// safe: should the original dispatcher come back to life and try to write its
// result, the token no longer matches and its update is dropped.
func (s *Store) UnlockStuckEvents(ctx context.Context, lockExpiry time.Duration) error {
	filter := bson.M{
		collectionFieldStatus: outbox.StatusInProgress, // Only events currently in progress.
		opExpr: bson.M{opLt: bson.A{
			"$" + collectionFieldLockedOn,
			bson.M{opSubtract: bson.A{serverNow, lockExpiry.Milliseconds()}},
		}},
	}
	// Aggregation-pipeline update so $$NOW-derived semantics stay server-side; the
	// lock is cleared (null) to mark the event unlocked.
	update := mongo.Pipeline{bson.D{{Key: opSet, Value: bson.M{
		collectionFieldStatus:        outbox.StatusPending,
		collectionFieldLockedOn:      nil,
		collectionFieldLockToken:     nil,
		collectionFieldNextAttemptAt: nil,
	}}}}
	if _, err := s.collection.UpdateMany(ctx, filter, update); err != nil {
		return coreerrs.WrapOperation(err, "unlock stuck events in MongoDB")
	}
	return nil
}

// sweepBoundMargin widens the client-side time bound of the expiry and
// retention sweeps, so a client clock behind the server's by less than this
// still reaches every due event in the same sweep. The bound only lets an
// index limit the scan; the server clock ($$NOW) still decides.
const sweepBoundMargin = time.Hour

// DeleteProcessedEvents removes processed events (sent, skipped, or expired) whose
// publication is older than olderThan, evaluated server-side ($$NOW - olderThan).
// A client bound on published_at, sweepBoundMargin later than the cutoff, lets
// the (status, published_at) index limit the sweep to events near or past it.
func (s *Store) DeleteProcessedEvents(ctx context.Context, olderThan time.Duration) error {
	filter := bson.M{
		collectionFieldStatus: bson.M{opIn: []outbox.Status{
			outbox.StatusSent, outbox.StatusSkipped, outbox.StatusExpired,
		}},
		collectionFieldPublishedAt: bson.M{opLt: time.Now().Add(-olderThan + sweepBoundMargin)},
		opExpr: bson.M{opLt: bson.A{
			"$" + collectionFieldPublishedAt,
			bson.M{opSubtract: bson.A{serverNow, olderThan.Milliseconds()}},
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
// Every write is fenced by the event's lock token: the filter requires the
// stored token to still match the one the dispatcher was handed at fetch time.
// If the unlock sweeper reclaimed the event and another worker picked it up,
// the token has been replaced and this (now stale) write is silently dropped
// instead of overwriting the newer attempt's result.
//
// last_attempt_on is stamped with the server clock ($$NOW) rather than a
// client timestamp, and next_attempt_at is computed as $$NOW plus the
// outbox-supplied RetryAfter duration, so the retry gate in
// FetchUnprocessedEvents (which also compares against $$NOW) stays clock-skew
// safe regardless of which instance dispatched the event. The lock is cleared
// so the event leaves the in-progress set.
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
			// Server-stamped, like the deadline it is later compared against.
			// A client timestamp here would make retention skew-sensitive: a
			// host whose clock runs ahead of the database writes a
			// published_at in the database's future, and the retention sweep —
			// which compares against $$NOW — keeps the event past its window.
			collectionFieldPublishedAt:   completedAtExpr(ev.PublishedAt),
			collectionFieldNextAttemptAt: nextAttemptExpr(ev.RetryAfter),
			// $literal preserves error strings starting with a dollar sign.
			// A nil *string encodes as null, which clears the previous error
			// on a successful attempt and records it on a failed one.
			collectionFieldLastError: bson.M{"$literal": ev.LastError},
			// Releasing the token ends this lease. A retry gets a fresh
			// token from the next fetch.
			collectionFieldLockToken: nil,
		}

		filter := bson.D{
			{Key: collectionFieldId, Value: ev.Id},
			{Key: collectionFieldLockToken, Value: ev.LockToken},
		}
		// Aggregation-pipeline update is required to reference $$NOW.
		model := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(mongo.Pipeline{bson.D{{Key: opSet, Value: set}}})
		writeModels = append(writeModels, model)
	}

	if _, err := s.collection.BulkWrite(ctx, writeModels); err != nil {
		return coreerrs.Wrap(err, "MongoDB BulkWrite failed for UpdateEvents")
	}
	return nil
}

// completedAtExpr renders the completion timestamp as the server's own clock
// when the event reached a processed state, and nil otherwise. Only the fact
// that the caller set a completion time is taken from the client; the instant
// itself comes from the database, so retention compares two readings of one
// clock. [Store.ExpireEvents] stamps the same field the same way.
func completedAtExpr(publishedAt time.Time) any {
	if publishedAt.IsZero() {
		return nil
	}
	return serverNow
}

// nextAttemptExpr renders the retry deadline as a server-side expression:
// $$NOW + retryAfter. A non-positive delay yields nil, which both clears any
// previous deadline and marks the event immediately eligible — the right
// meaning for terminal and successfully dispatched events alike.
func nextAttemptExpr(retryAfter time.Duration) any {
	if retryAfter <= 0 {
		return nil
	}
	return bson.M{"$add": bson.A{serverNow, retryAfter.Milliseconds()}}
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
		// The BSON field "event" stores the public Event.Payload value.
		doc := event{
			Id:        ev.Id,
			Status:    string(ev.Status),
			Event:     ev.Payload,
			CreatedAt: ev.CreatedAt.UTC(),
			Topic:     ev.Key,
			Attempts:  ev.Attempts,
			LastError: ev.LastError,
			ExpiresAt: nullableDate(ev.ExpiresAt),
			Seq:       bson.NewObjectID(),
			// PublishedAt, LastAttemptOn, LockedOn are nil for new events.
		}
		insertModels = append(insertModels, mongo.NewInsertOneModel().SetDocument(doc))
	}

	if _, err := s.collection.BulkWrite(ctx, insertModels); err != nil {
		return coreerrs.Wrap(err, "MongoDB BulkWrite failed for SaveEvents")
	}
	return nil
}

// FetchUnprocessedEvents retrieves pending events, and failed events whose
// backoff has elapsed, then locks them for processing. The batch is sorted by
// created_at ascending, as [outbox.Storage] requires, with seq keeping events
// saved together in insertion order.
//
// The retry gate ($next_attempt_at <= $$NOW) and the not-yet-expired predicate
// ($expires_at > $$NOW) are evaluated against the MongoDB server clock, and the
// lock timestamp is written as $$NOW — never a client time — so concurrent
// instances with skewed wall clocks agree on eligibility and lock freshness.
//
// Each locked event is stamped with a fresh lock token that [Store.UpdateEvents]
// uses to fence stale writes.
//
// A batchSize of 0 is refused with an error matching
// coreerrs.ErrInvalidArgument: MongoDB reads a limit of 0 as no limit, which
// would lock every ready event at once.
func (s *Store) FetchUnprocessedEvents(ctx context.Context, batchSize uint32) ([]outbox.Event, error) {
	if batchSize == 0 {
		return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "outbox fetch batch size must be positive")
	}

	// Exclude events whose ExpiresAt has passed. Documents without expiration omit
	// the field entirely (pointer + omitempty), so $exists:false admits them.
	notExpiredFilter := bson.M{opOr: bson.A{
		bson.M{collectionFieldExpiresAt: bson.M{opExists: false}},
		bson.M{opExpr: bson.M{"$gt": bson.A{"$" + collectionFieldExpiresAt, serverNow}}},
	}}

	// A failed event carries its own backoff deadline, written server-side when
	// the attempt failed. A missing or null deadline means "eligible now": a
	// never-attempted event omits the field and the unlock sweep writes null.
	backoffElapsed := bson.M{opOr: bson.A{
		bson.M{collectionFieldNextAttemptAt: bson.M{opExists: false}},
		bson.M{collectionFieldNextAttemptAt: nil},
		bson.M{opExpr: bson.M{opLte: bson.A{"$" + collectionFieldNextAttemptAt, serverNow}}},
	}}

	// One query per ready status, each an equality on status read through
	// fetchIndexKeys in fetch order, merged below. A single $or over both
	// statuses cannot take its sort from one index, so the server would sort
	// the whole backlog to return a batch.
	filters := [...]bson.M{
		{"$and": bson.A{bson.M{collectionFieldStatus: outbox.StatusPending}, notExpiredFilter}},
		{"$and": bson.A{bson.M{collectionFieldStatus: outbox.StatusFailed}, backoffElapsed, notExpiredFilter}},
	}
	findOpts := mongoOptions.Find().
		SetSort(bson.D{{Key: collectionFieldCreatedAt, Value: 1}, {Key: collectionFieldSeq, Value: 1}}).
		SetLimit(int64(batchSize)).
		SetHint(fetchIndexKeys)

	var (
		mongoEvents []event
		// lockedAt is informational only: the authoritative lock timestamp is the
		// server's $$NOW written below. The returned LockedOn is cleared by the
		// dispatcher before the event is used, so an approximate client value is fine.
		lockedAt time.Time
		// lockToken identifies this lease. One token per fetch is enough to
		// fence: the transaction below guarantees no two fetches can claim the
		// same document, so distinct leases always carry distinct tokens.
		lockToken string
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
		// A re-executed attempt must also mint a fresh token, or a partially
		// applied earlier attempt could leave documents fenced to a token we
		// no longer report to the caller.
		mongoEvents = nil
		lockedAt = time.Now().UTC()
		lockToken = uuid.New().String()

		var batches [2][]event
		for i, filter := range filters {
			if txErr := s.findAll(sessCtx, filter, findOpts, &batches[i]); txErr != nil {
				return nil, txErr
			}
		}
		mongoEvents = mergeFetchBatches(batches[0], batches[1], int(batchSize))

		if len(mongoEvents) == 0 {
			return 0, nil // No events to process
		}

		// Collect IDs of fetched events to lock them.
		idsToLock := slices.Collect(coreslices.Map(mongoEvents, func(e event) string { return e.Id }))

		// Lock via aggregation-pipeline update so locked_on carries the server clock.
		update := mongo.Pipeline{bson.D{{Key: opSet, Value: bson.M{
			collectionFieldStatus:    outbox.StatusInProgress,
			collectionFieldLockedOn:  serverNow,
			collectionFieldLockToken: lockToken,
		}}}}
		if _, txErr := s.collection.UpdateMany(sessCtx, bson.M{collectionFieldId: bson.M{opIn: idsToLock}}, update); txErr != nil {
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
			LockToken:     lockToken,
		}
	})), nil
}

// findAll runs one fetch query and decodes every document into out.
func (s *Store) findAll(ctx context.Context, filter bson.M, opts *mongoOptions.FindOptionsBuilder, out *[]event) error {
	cursor, err := s.collection.Find(ctx, filter, opts)
	if err != nil {
		return coreerrs.Wrap(err, "MongoDB Find failed in FetchUnprocessedEvents")
	}
	defer func() { _ = cursor.Close(ctx) }()
	return coreerrs.Wrap(cursor.All(ctx, out), "MongoDB cursor.All failed in FetchUnprocessedEvents")
}

// mergeFetchBatches merges two batches already in fetch order into one of at
// most limit events, in fetch order.
func mergeFetchBatches(a, b []event, limit int) []event {
	out := make([]event, 0, min(len(a)+len(b), limit))
	for len(out) < limit && (len(a) > 0 || len(b) > 0) {
		if len(b) == 0 || (len(a) > 0 && compareFetchOrder(a[0], b[0]) <= 0) {
			out, a = append(out, a[0]), a[1:]
		} else {
			out, b = append(out, b[0]), b[1:]
		}
	}
	return out
}

// compareFetchOrder orders events as the fetch sort does: created_at, then
// seq. A missing seq decodes as the zero ObjectID, which sorts first, as an
// absent field does on the server.
func compareFetchOrder(a, b event) int {
	if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
		return c
	}
	return bytes.Compare(a.Seq[:], b.Seq[:])
}

// Stats returns the backlog snapshot backing the outbox gauges.
//
// The counts and the age of the oldest waiting event are produced by a single
// aggregation evaluated on the server, so the age is measured against the same
// clock that decides eligibility and cannot be skewed by the caller's host.
func (s *Store) Stats(ctx context.Context) (outbox.Stats, error) {
	waiting := bson.A{outbox.StatusPending, outbox.StatusFailed}
	deadLettered := bson.A{outbox.StatusMaxAttemptReached, outbox.StatusRejected}

	// Derived from the two groups above so the match and the counters cannot
	// drift apart when a status is added. Terminal successes (sent, skipped,
	// expired) are excluded here, which is what keeps this a backlog query
	// rather than a full-collection scan.
	counted := make(bson.A, 0, 1+len(waiting)+len(deadLettered))
	counted = append(counted, outbox.StatusInProgress)
	counted = append(counted, waiting...)
	counted = append(counted, deadLettered...)

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{collectionFieldStatus: bson.M{opIn: counted}}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id": nil,
			"pending": bson.M{opSum: bson.M{opCond: bson.A{
				bson.M{opIn: bson.A{"$" + collectionFieldStatus, waiting}}, 1, 0,
			}}},
			"in_progress": bson.M{opSum: bson.M{opCond: bson.A{
				bson.M{opEq: bson.A{"$" + collectionFieldStatus, outbox.StatusInProgress}}, 1, 0,
			}}},
			"dead_lettered": bson.M{opSum: bson.M{opCond: bson.A{
				bson.M{opIn: bson.A{"$" + collectionFieldStatus, deadLettered}}, 1, 0,
			}}},
			// Oldest creation time among the events still waiting; null when
			// none are, which $min yields naturally.
			"oldest_waiting": bson.M{"$min": bson.M{opCond: bson.A{
				bson.M{opIn: bson.A{"$" + collectionFieldStatus, waiting}},
				"$" + collectionFieldCreatedAt,
				nil,
			}}},
		}}},
		bson.D{{Key: "$project", Value: bson.M{
			"pending":       1,
			"in_progress":   1,
			"dead_lettered": 1,
			// Age in milliseconds against the server clock; 0 when nothing waits.
			"oldest_pending_age_ms": bson.M{opCond: bson.A{
				bson.M{opEq: bson.A{"$oldest_waiting", nil}},
				0,
				bson.M{opSubtract: bson.A{serverNow, "$oldest_waiting"}},
			}},
		}}},
	}

	cursor, err := s.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return outbox.Stats{}, coreerrs.WrapOperation(err, "aggregate outbox stats in MongoDB")
	}
	defer func() { _ = cursor.Close(ctx) }()

	var rows []struct {
		Pending            int64 `bson:"pending"`
		InProgress         int64 `bson:"in_progress"`
		DeadLettered       int64 `bson:"dead_lettered"`
		OldestPendingAgeMs int64 `bson:"oldest_pending_age_ms"`
	}
	if err = cursor.All(ctx, &rows); err != nil {
		return outbox.Stats{}, coreerrs.WrapOperation(err, "decode outbox stats from MongoDB")
	}
	if len(rows) == 0 {
		return outbox.Stats{}, nil // Empty collection: every counter is legitimately zero.
	}

	return outbox.Stats{
		Pending:          rows[0].Pending,
		InProgress:       rows[0].InProgress,
		DeadLettered:     rows[0].DeadLettered,
		OldestPendingAge: time.Duration(rows[0].OldestPendingAgeMs) * time.Millisecond,
	}, nil
}

// ExpireEvents marks pending or failed events whose ExpiresAt has passed as expired.
// The deadline is evaluated against the server clock ($expires_at <= $$NOW), and
// published_at is stamped with $$NOW so subsequent cleanup is clock-skew safe too.
// A client bound of now plus sweepBoundMargin on expires_at lets the
// (status, expires_at) index limit the sweep to events expiring around now
// instead of the whole pending and failed backlog; $$NOW still decides.
func (s *Store) ExpireEvents(ctx context.Context) (int64, error) {
	filter := bson.M{
		collectionFieldStatus:    bson.M{opIn: []outbox.Status{outbox.StatusPending, outbox.StatusFailed}},
		collectionFieldExpiresAt: bson.M{opLte: time.Now().Add(sweepBoundMargin)},
		opExpr:                   bson.M{opLte: bson.A{"$" + collectionFieldExpiresAt, serverNow}},
	}
	update := mongo.Pipeline{bson.D{{Key: opSet, Value: bson.M{
		collectionFieldStatus:        outbox.StatusExpired,
		collectionFieldPublishedAt:   serverNow,
		collectionFieldLockedOn:      nil,
		collectionFieldLockToken:     nil,
		collectionFieldNextAttemptAt: nil,
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

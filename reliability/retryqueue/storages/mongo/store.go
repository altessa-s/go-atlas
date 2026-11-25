// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/internal/retryqueuefields"
	"github.com/altessa-s/go-atlas/reliability/retryqueue"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	datamongo "github.com/altessa-s/go-atlas/store/mongo"
	mongoindexes "github.com/altessa-s/go-atlas/store/mongo/indexes"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Index names created by [Store.EnsureIndexes].
const (
	// IndexPendingNextAttempt serves ListDue and WakeDue.
	IndexPendingNextAttempt = "idx_pending_next_attempt"
	// IndexStateSince serves ListByState and Requeue.
	IndexStateSince = "idx_state_since"
	// IndexResolvedTTL purges resolved items after the retention.
	IndexResolvedTTL = "ttl_resolved"
)

// ErrInvalidFlagField reports a flag field name that is not a plain top-level
// field name, collides with a field the store owns, or is shared by both flag
// fields.
var ErrInvalidFlagField = retryqueuefields.ErrInvalidFlagField

// ErrInvalidPayload reports a payload type whose document fields are not all
// known statically (it is not a struct or a struct pointer, or it inlines a
// map), or whose fields collide with a field the store owns or a flag field.
// The driver would otherwise drop a colliding payload field silently, and a
// map would mix the store's flag fields into the payload on decode.
var ErrInvalidPayload = errors.New("retryqueue mongo: invalid payload type")

// markAttempts bounds the upserts of Mark: the first-writer race loses at most
// once, and the repeated upsert then returns the winner's item.
const markAttempts = 2

// reopenAttempts bounds the replace-then-mark rounds of Reopen when a writer
// inserts an older or closed item between the two statements.
const reopenAttempts = 2

// Query and update operators the store reuses.
const (
	opExpr  = "$expr"  // Query predicate evaluated as an expression.
	opOr    = "$or"    // Logical disjunction.
	opNe    = "$ne"    // Inequality comparison.
	opSet   = "$set"   // Field assignment.
	opUnset = "$unset" // Field removal.
)

var (
	_ retryqueue.AttemptStorage[struct{}] = (*Store[struct{}])(nil)
	_ retryqueue.Mover                    = (*Store[struct{}])(nil)
)

var _ retryqueue.AttemptStorage[struct{}] = (*Store[struct{}])(nil)

// Store is the MongoDB-backed [retryqueue.Storage]: one document per item,
// with the key as _id and the payload P inlined. P must be a struct or a
// struct pointer, without an inlined map, whose fields do not use the store's
// or the flag field names; decoding a document into P then ignores the store's
// fields.
type Store[P any] struct {
	coll        *mongo.Collection
	flagField   string
	reasonField string
	retention   time.Duration
}

// New returns a Store over coll. Call [Store.EnsureIndexes] once at startup.
// It performs no I/O. It fails when coll is nil, with [ErrInvalidFlagField]
// when a flag field name starts with '$', contains '.', is reserved by the
// store, or is shared by both flag fields, and with [ErrInvalidPayload] when P
// is not a struct or struct pointer, inlines a map, or has a field that
// collides with a store or flag field.
func New[P any](coll *mongo.Collection, opts ...Option) (*Store[P], error) {
	if coll == nil {
		return nil, errors.New("retryqueue mongo: collection is required")
	}
	o := newOptions(opts...)
	// The TTL index expiry is rounded up to whole seconds; bound the
	// retention first so rounding cannot overflow and a capped expiry never
	// purges items early.
	if o.resolvedRetention > math.MaxInt32*time.Second {
		return nil, errors.New("retryqueue mongo: resolved retention is too long")
	}
	if !retryqueuefields.ValidFlag(o.flagField, o.flagReasonField) || !retryqueuefields.ValidFlag(o.flagReasonField, o.flagField) {
		return nil, ErrInvalidFlagField
	}
	fields, ok := payloadFields(reflect.TypeFor[P]())
	if !ok {
		return nil, ErrInvalidPayload
	}
	for _, f := range fields {
		if f == o.flagField || f == o.flagReasonField || retryqueuefields.Reserved(f) {
			return nil, ErrInvalidPayload
		}
	}
	return &Store[P]{coll: coll, flagField: o.flagField, reasonField: o.flagReasonField, retention: o.resolvedRetention}, nil
}

// EnsureIndexes creates the due-list and per-state indexes, plus the TTL index
// on resolved items when a retention is configured. It is idempotent; changing
// the retention of an existing TTL index makes MongoDB reject it (drop the
// index first).
func (s *Store[P]) EnsureIndexes(ctx context.Context) error {
	models := []mongo.IndexModel{
		{
			// Only pending items are indexed; others are never listed as due.
			Keys: bson.D{{Key: fNextAttempt, Value: 1}, {Key: fID, Value: 1}},
			Options: mongoOptions.Index().SetName(IndexPendingNextAttempt).
				SetPartialFilterExpression(bson.M{fState: string(retryqueue.StatePending)}),
		},
		{
			Keys:    bson.D{{Key: fState, Value: 1}, {Key: fSince, Value: 1}, {Key: fID, Value: 1}},
			Options: mongoOptions.Index().SetName(IndexStateSince),
		},
	}
	if s.retention > 0 {
		models = append(models, mongo.IndexModel{
			// The partial filter keeps a pending or expired item out of the
			// TTL index even if it carried a resolvedAt.
			Keys: bson.D{{Key: fResolvedAt, Value: 1}},
			Options: mongoOptions.Index().SetName(IndexResolvedTTL).
				SetExpireAfterSeconds(ttlSeconds(s.retention)).
				SetPartialFilterExpression(bson.M{fState: string(retryqueue.StateResolved)}),
		})
	}
	if err := mongoindexes.Ensure(ctx, s.coll, models); err != nil {
		return coreerrs.WrapOperation(err, "create retry queue indexes")
	}
	return nil
}

// ttlSeconds converts d, which [New] bounds to the largest TTL MongoDB
// accepts, to whole seconds, rounded up so a resolved item is never purged
// before the retention has passed.
func ttlSeconds(d time.Duration) int32 {
	return int32((d + time.Second - 1) / time.Second)
}

// Mark implements [retryqueue.Storage] with an insert-only upsert. Two
// concurrent first marks of the same key can both try to insert; the loser
// gets a duplicate-key error and repeats the upsert, which then returns the
// winner's item (or inserts again if it was deleted meanwhile). Inside a
// transaction the upsert runs once: the duplicate-key error has aborted the
// transaction, so only a retry of the whole transaction can recover.
func (s *Store[P]) Mark(ctx context.Context, item retryqueue.Item[P]) (retryqueue.Item[P], error) {
	filter := bson.M{fID: item.Key}
	update := bson.M{"$setOnInsert": freshDoc(item)}
	opts := mongoOptions.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(mongoOptions.After)

	var out retryqueue.Item[P]
	err := datamongo.UpsertRetryingDuplicate(ctx, markAttempts, func(ctx context.Context) error {
		var err error
		out, err = s.decodeOne(s.coll.FindOneAndUpdate(ctx, filter, update, opts))
		return err
	})
	if err != nil {
		return retryqueue.Item[P]{}, coreerrs.WrapOperation(err, "mark retry item")
	}
	return out, nil
}

// Reopen implements [retryqueue.Storage] by replacing the matched document
// with a fresh one in a single write, so every stored payload field is
// replaced (a field the new payload's encoding omits is gone) and nothing of
// the previous generation survives. When no document matches, it falls back
// to [Store.Mark]. If Mark returns an older generation or a closed item at
// the same generation, it retries the replacement, for at most two rounds.
// Persistent contention returns the last stored item; the caller compares
// generations per the Storage contract.
func (s *Store[P]) Reopen(ctx context.Context, item retryqueue.Item[P]) (retryqueue.Item[P], error) {
	// A missing generation belongs to generation zero.
	generation := documentGeneration()
	filter := bson.M{fID: item.Key, opOr: bson.A{
		bson.M{opExpr: bson.M{"$lt": bson.A{generation, item.Generation}}},
		bson.M{fGeneration: genEq(item.Generation), fState: bson.M{opNe: string(retryqueue.StatePending)}},
	}}
	// Replace the entire document, preserving a strictly increasing age fence
	// when the generation is reused. Payload strings are opaque BSON values.
	since := bson.M{"$cond": bson.A{
		bson.M{"$eq": bson.A{generation, item.Generation}}, nextSince(item.Since), item.Since.UTC(),
	}}
	update := mongo.Pipeline{
		{{Key: "$replaceWith", Value: bson.M{"$mergeObjects": bson.A{
			bson.M{"$literal": freshDoc(item)}, bson.M{fSince: since},
		}}}},
	}
	var out retryqueue.Item[P]
	for range reopenAttempts {
		got, err := s.decodeOne(s.coll.FindOneAndUpdate(ctx, filter, update,
			mongoOptions.FindOneAndUpdate().SetReturnDocument(mongoOptions.After)))
		switch {
		case err == nil:
			return got, nil
		case !errors.Is(err, mongo.ErrNoDocuments):
			return retryqueue.Item[P]{}, coreerrs.WrapOperation(err, "reopen retry item")
		}
		out, err = s.Mark(ctx, item)
		if err != nil {
			return retryqueue.Item[P]{}, err
		}
		if out.Generation > item.Generation || (out.Generation == item.Generation && out.State == retryqueue.StatePending) {
			return out, nil
		}
	}
	return out, nil
}

// ListDue implements [retryqueue.Storage].
func (s *Store[P]) ListDue(ctx context.Context, now time.Time, limit int) ([]retryqueue.Item[P], error) {
	filter := bson.M{fState: string(retryqueue.StatePending), fNextAttempt: bson.M{"$lte": now.UTC()}}
	sort := bson.D{{Key: fNextAttempt, Value: 1}, {Key: fID, Value: 1}}
	return s.find(ctx, filter, sort, limit, "list due retry items")
}

// WakeDue implements [retryqueue.Storage] with one UpdateMany over the pending
// items scheduled after now.
func (s *Store[P]) WakeDue(ctx context.Context, now time.Time) error {
	_, err := s.coll.UpdateMany(ctx,
		bson.M{fState: string(retryqueue.StatePending), fNextAttempt: bson.M{"$gt": now.UTC()}},
		bson.M{opSet: bson.M{fNextAttempt: now.UTC()}},
	)
	if err != nil {
		return coreerrs.WrapOperation(err, "wake due retry items")
	}
	return nil
}

// Reschedule implements [retryqueue.Storage].
func (s *Store[P]) Reschedule(ctx context.Context, key string, generation int64, next time.Time) error {
	return s.reschedule(ctx, key, generation, nil, next)
}

// RescheduleAttempt implements [retryqueue.AttemptStorage].
func (s *Store[P]) RescheduleAttempt(ctx context.Context, attempt retryqueue.Attempt, next time.Time) error {
	return s.reschedule(ctx, attempt.Key, attempt.Generation, &attempt.Since, next)
}

func (s *Store[P]) reschedule(ctx context.Context, key string, generation int64, since *time.Time, next time.Time) error {
	filter := bson.M{fID: key, fState: string(retryqueue.StatePending), fGeneration: genEq(generation)}
	if since != nil {
		filter[fSince] = since.UTC()
	}
	_, err := s.coll.UpdateOne(ctx,
		filter,
		bson.M{"$inc": bson.M{fAttempts: 1}, opSet: bson.M{fNextAttempt: next.UTC()}},
	)
	if err != nil {
		return coreerrs.WrapOperation(err, "reschedule retry item")
	}
	return nil
}

// Expire implements [retryqueue.Storage].
func (s *Store[P]) Expire(ctx context.Context, key string, generation int64, since, at time.Time) (bool, error) {
	res, err := s.coll.UpdateOne(ctx,
		bson.M{fID: key, fState: string(retryqueue.StatePending), fGeneration: genEq(generation), fSince: since.UTC()},
		bson.M{opSet: bson.M{fState: string(retryqueue.StateExpired), fExpiredAt: at.UTC()}},
	)
	if err != nil {
		return false, coreerrs.WrapOperation(err, "expire retry item")
	}
	return res.ModifiedCount > 0, nil
}

// Resolve implements [retryqueue.Storage].
func (s *Store[P]) Resolve(ctx context.Context, key string, generation int64, at time.Time) error {
	return s.resolve(ctx, key, generation, nil, at)
}

// ResolveAttempt implements [retryqueue.AttemptStorage].
func (s *Store[P]) ResolveAttempt(ctx context.Context, attempt retryqueue.Attempt, at time.Time) error {
	return s.resolve(ctx, attempt.Key, attempt.Generation, &attempt.Since, at)
}

func (s *Store[P]) resolve(ctx context.Context, key string, generation int64, since *time.Time, at time.Time) error {
	filter := bson.M{fID: key, fState: bson.M{opNe: string(retryqueue.StateResolved)}, fGeneration: genEq(generation)}
	if since != nil {
		filter[fSince] = since.UTC()
	}
	_, err := s.coll.UpdateOne(ctx,
		filter,
		bson.M{
			opSet:   bson.M{fState: string(retryqueue.StateResolved), fResolvedAt: at.UTC()},
			opUnset: bson.M{fExpiredAt: "", s.flagField: "", s.reasonField: ""},
		},
	)
	if err != nil {
		return coreerrs.WrapOperation(err, "resolve retry item")
	}
	return nil
}

// Flag implements [retryqueue.Storage].
func (s *Store[P]) Flag(ctx context.Context, key string, generation int64, reason string) (bool, error) {
	return s.flag(ctx, key, generation, nil, reason)
}

// FlagAttempt implements [retryqueue.AttemptStorage].
func (s *Store[P]) FlagAttempt(ctx context.Context, attempt retryqueue.Attempt, reason string) (bool, error) {
	return s.flag(ctx, attempt.Key, attempt.Generation, &attempt.Since, reason)
}

func (s *Store[P]) flag(ctx context.Context, key string, generation int64, since *time.Time, reason string) (bool, error) {
	filter := bson.M{fID: key, fState: string(retryqueue.StatePending), fGeneration: genEq(generation), opOr: bson.A{
		bson.M{s.flagField: bson.M{opNe: true}},
		bson.M{s.reasonField: bson.M{opNe: reason}},
	}}
	if since != nil {
		filter[fSince] = since.UTC()
	}
	res, err := s.coll.UpdateOne(ctx,
		filter,
		bson.M{opSet: bson.M{s.flagField: true, s.reasonField: reason}},
	)
	if err != nil {
		return false, coreerrs.WrapOperation(err, "flag retry item")
	}
	return res.ModifiedCount > 0, nil
}

// ClearFlag implements [retryqueue.Storage].
func (s *Store[P]) ClearFlag(ctx context.Context, key string, generation int64) error {
	return s.clearFlag(ctx, key, generation, nil)
}

// ClearAttemptFlag implements [retryqueue.AttemptStorage].
func (s *Store[P]) ClearAttemptFlag(ctx context.Context, attempt retryqueue.Attempt) error {
	return s.clearFlag(ctx, attempt.Key, attempt.Generation, &attempt.Since)
}

func (s *Store[P]) clearFlag(ctx context.Context, key string, generation int64, since *time.Time) error {
	filter := bson.M{fID: key, fGeneration: genEq(generation), s.flagField: true}
	if since != nil {
		filter[fSince] = since.UTC()
	}
	_, err := s.coll.UpdateOne(ctx,
		filter,
		bson.M{opUnset: bson.M{s.flagField: "", s.reasonField: ""}},
	)
	if err != nil {
		return coreerrs.WrapOperation(err, "clear retry item flag")
	}
	return nil
}

// DeleteObsolete implements [retryqueue.Storage]: a conditional delete, then,
// when nothing was deleted, a look-up of a newer item.
func (s *Store[P]) DeleteObsolete(ctx context.Context, key string, generation int64) (bool, error) {
	res, err := s.coll.DeleteOne(ctx, bson.M{fID: key, opExpr: bson.M{"$lte": bson.A{documentGeneration(), generation}}})
	if err != nil {
		return false, coreerrs.WrapOperation(err, "delete obsolete retry item")
	}
	if res.DeletedCount > 0 {
		return false, nil
	}
	err = s.coll.FindOne(ctx, bson.M{fID: key, opExpr: bson.M{"$gt": bson.A{documentGeneration(), generation}}},
		mongoOptions.FindOne().SetProjection(bson.M{fID: 1})).Err()
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return false, nil
	case err != nil:
		return false, coreerrs.WrapOperation(err, "look up newer retry item")
	}
	return true, nil
}

// documentGeneration treats legacy documents without a generation as generation zero.
func documentGeneration() bson.M {
	return bson.M{"$ifNull": bson.A{"$" + fGeneration, int64(0)}}
}

// Delete implements [retryqueue.Storage].
func (s *Store[P]) Delete(ctx context.Context, key string) error {
	if _, err := s.coll.DeleteOne(ctx, bson.M{fID: key}); err != nil {
		return coreerrs.WrapOperation(err, "delete retry item")
	}
	return nil
}

// ListByState implements [retryqueue.Storage].
func (s *Store[P]) ListByState(ctx context.Context, state retryqueue.State, limit int) ([]retryqueue.Item[P], error) {
	sort := bson.D{{Key: fSince, Value: 1}, {Key: fID, Value: 1}}
	return s.find(ctx, bson.M{fState: string(state)}, sort, limit, "list retry items by state")
}

// Requeue implements [retryqueue.Storage].
func (s *Store[P]) Requeue(ctx context.Context, now time.Time, keys ...string) (int, error) {
	filter := bson.M{fState: string(retryqueue.StateExpired)}
	if len(keys) > 0 {
		filter[fID] = bson.M{"$in": keys}
	}
	res, err := s.coll.UpdateMany(ctx, filter, mongo.Pipeline{
		{{Key: opSet, Value: bson.M{
			fState: string(retryqueue.StatePending), fSince: nextSince(now), fNextAttempt: now.UTC(), fAttempts: 0,
			fExpiredAt: "$$REMOVE",
		}}},
	})
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "requeue retry items")
	}
	return int(res.ModifiedCount), nil
}

// nextSince renders a new age later than the previous BSON date, even if the
// caller supplied the same millisecond or its clock moved backwards.
func nextSince(now time.Time) bson.M {
	return bson.M{"$max": bson.A{now.UTC(), bson.M{"$add": bson.A{"$" + fSince, 1}}}}
}

// Move implements [retryqueue.Mover] in a transaction: it looks both keys
// up, deletes the document of from (FindOneAndDelete) and inserts it under
// to, every stored field kept as stored (fields the store does not know and
// the flag fields included). When ctx carries a running transaction ([datamongo.InTransaction])
// the move joins it and commits or aborts with it; otherwise it runs in its
// own transaction on a new session of the collection's client, which needs a
// replica set or a sharded cluster. Both refusals are found before any write,
// so they leave an enclosing transaction usable; only a concurrent insert of
// to between the check and the insert fails the insert, and the duplicate-key
// error, reported as [retryqueue.ErrItemExists], aborts the transaction.
func (s *Store[P]) Move(ctx context.Context, from, to string) error {
	if datamongo.InTransaction(ctx) {
		return s.move(ctx, from, to)
	}
	sess, err := s.coll.Database().Client().StartSession()
	if err != nil {
		return coreerrs.WrapOperation(err, "start session for moving retry item")
	}
	defer sess.EndSession(context.WithoutCancel(ctx))
	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, s.move(ctx, from, to)
	})
	return err
}

// move moves the document of from to to within the transaction ctx carries.
// It looks both keys up before writing: from first, so a missing source is
// reported as such whatever to holds, as the memory store does.
func (s *Store[P]) move(ctx context.Context, from, to string) error {
	found, err := s.exists(ctx, from)
	switch {
	case err != nil:
		return coreerrs.WrapOperation(err, "look up retry item to move")
	case !found:
		return fmt.Errorf("move %q: %w", from, retryqueue.ErrItemNotFound)
	case from == to:
		return nil
	}
	taken, err := s.exists(ctx, to)
	switch {
	case err != nil:
		return coreerrs.WrapOperation(err, "look up retry item move target")
	case taken:
		return fmt.Errorf("move %q to %q: %w", from, to, retryqueue.ErrItemExists)
	}
	raw, err := s.coll.FindOneAndDelete(ctx, bson.M{fID: from}).Raw()
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return fmt.Errorf("move %q: %w", from, retryqueue.ErrItemNotFound)
	case err != nil:
		return coreerrs.WrapOperation(err, "delete retry item to move")
	}
	doc, err := rekeyed(raw, to)
	if err != nil {
		return coreerrs.WrapOperation(err, "decode retry item to move")
	}
	if _, err := s.coll.InsertOne(ctx, doc); err != nil {
		if dup, _ := datamongo.IsErrorDuplicate(err); dup {
			return fmt.Errorf("move %q to %q: %w: %w", from, to, retryqueue.ErrItemExists, err)
		}
		return coreerrs.WrapOperation(err, "insert moved retry item")
	}
	return nil
}

// exists reports whether key has a document.
func (s *Store[P]) exists(ctx context.Context, key string) (bool, error) {
	err := s.coll.FindOne(ctx, bson.M{fID: key}, mongoOptions.FindOne().SetProjection(bson.M{fID: 1})).Err()
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

// rekeyed returns raw with its _id replaced by key and every other field as
// stored, in the stored order.
func rekeyed(raw bson.Raw, key string) (bson.D, error) {
	elems, err := raw.Elements()
	if err != nil {
		return nil, err
	}
	doc := make(bson.D, 0, len(elems))
	for _, e := range elems {
		if e.Key() == fID {
			doc = append(doc, bson.E{Key: fID, Value: key})
			continue
		}
		doc = append(doc, bson.E{Key: e.Key(), Value: e.Value()})
	}
	return doc, nil
}

// decodeOne decodes the document of res, returning its error as is (for
// example mongo.ErrNoDocuments or a duplicate-key error).
func (s *Store[P]) decodeOne(res *mongo.SingleResult) (retryqueue.Item[P], error) {
	var d itemDoc[P]
	if err := res.Decode(&d); err != nil {
		return retryqueue.Item[P]{}, err
	}
	raw, err := res.Raw()
	if err != nil {
		return retryqueue.Item[P]{}, err
	}
	return d.toItem(raw, s.flagField, s.reasonField), nil
}

// find runs a sorted, limited query and maps the documents to items; op names
// the operation in errors. A limit that is not positive means
// [retryqueue.DefaultListLimit].
func (s *Store[P]) find(ctx context.Context, filter bson.M, sort bson.D, limit int, op string) ([]retryqueue.Item[P], error) {
	if limit <= 0 {
		limit = retryqueue.DefaultListLimit
	}
	cur, err := s.coll.Find(ctx, filter, mongoOptions.Find().SetSort(sort).SetLimit(int64(limit)))
	if err != nil {
		return nil, coreerrs.WrapOperation(err, op)
	}
	defer func() { _ = cur.Close(context.WithoutCancel(ctx)) }()

	out := []retryqueue.Item[P]{}
	for cur.Next(ctx) {
		var d itemDoc[P]
		if err := cur.Decode(&d); err != nil {
			return nil, coreerrs.WrapOperation(err, "decode: "+op)
		}
		out = append(out, d.toItem(cur.Current, s.flagField, s.reasonField))
	}
	if err := cur.Err(); err != nil {
		return nil, coreerrs.WrapOperation(err, op)
	}
	return out, nil
}

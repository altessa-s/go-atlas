// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/retryqueue"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	datamongo "github.com/altessa-s/go-atlas/data/mongo"
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
var ErrInvalidFlagField = errors.New("retryqueue mongo: invalid flag field name")

// ErrInvalidPayload reports a payload type whose document fields are not all
// known statically (it is not a struct or a struct pointer, or it inlines a
// map), or whose fields collide with a field the store owns or a flag field.
// The driver would otherwise drop a colliding payload field silently, and a
// map would mix the store's flag fields into the payload on decode.
var ErrInvalidPayload = errors.New("retryqueue mongo: invalid payload type")

// markAttempts bounds the upserts of Mark: the first-writer race loses at most
// once, and the repeated upsert then returns the winner's item.
const markAttempts = 2

var _ retryqueue.Storage[struct{}] = (*Store[struct{}])(nil)

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
// It fails with [ErrInvalidFlagField] when a flag field name starts with '$',
// contains '.', is reserved by the store, or is shared by both flag fields,
// and with [ErrInvalidPayload] when P is not a struct or struct pointer, inlines
// a map, or has a field that collides with a store or flag field.
func New[P any](coll *mongo.Collection, opts ...Option) (*Store[P], error) {
	o := newOptions(opts...)
	if o.flagField == o.flagReasonField || !validField(o.flagField) || !validField(o.flagReasonField) {
		return nil, ErrInvalidFlagField
	}
	fields, ok := payloadFields(reflect.TypeFor[P]())
	if !ok {
		return nil, ErrInvalidPayload
	}
	for _, f := range fields {
		if f == o.flagField || f == o.flagReasonField || slices.Contains(reservedFields, f) {
			return nil, ErrInvalidPayload
		}
	}
	return &Store[P]{coll: coll, flagField: o.flagField, reasonField: o.flagReasonField, retention: o.resolvedRetention}, nil
}

// validField reports whether name is a plain top-level field the store does not
// own.
func validField(name string) bool {
	return !strings.HasPrefix(name, "$") && !strings.Contains(name, ".") && !slices.Contains(reservedFields, name)
}

// EnsureIndexes creates the due-list and per-state indexes, plus the TTL index
// on resolved items when a retention of at least one second is configured. It
// is idempotent; changing the retention of an existing TTL index makes MongoDB
// reject it (drop the index first).
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
	if s.retention >= time.Second {
		models = append(models, mongo.IndexModel{
			// The partial filter keeps a pending or expired item out of the
			// TTL index even if it carried a resolvedAt.
			Keys: bson.D{{Key: fResolvedAt, Value: 1}},
			Options: mongoOptions.Index().SetName(IndexResolvedTTL).
				SetExpireAfterSeconds(ttlSeconds(s.retention)).
				SetPartialFilterExpression(bson.M{fState: string(retryqueue.StateResolved)}),
		})
	}
	if err := datamongo.EnsureIndexes(ctx, s.coll, models); err != nil {
		return coreerrs.WrapOperation(err, "create retry queue indexes")
	}
	return nil
}

// ttlSeconds converts d to whole seconds, capped at the largest TTL MongoDB
// accepts.
func ttlSeconds(d time.Duration) int32 {
	return int32(min(int64(d/time.Second), math.MaxInt32))
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
// the previous generation survives.
func (s *Store[P]) Reopen(ctx context.Context, item retryqueue.Item[P]) (retryqueue.Item[P], error) {
	filter := bson.M{fID: item.Key, "$or": bson.A{
		bson.M{fGeneration: bson.M{"$lt": item.Generation}},
		bson.M{fGeneration: bson.M{"$exists": false}},
		bson.M{fGeneration: item.Generation, fState: bson.M{"$ne": string(retryqueue.StatePending)}},
	}}
	out, err := s.decodeOne(s.coll.FindOneAndReplace(ctx, filter, freshDoc(item),
		mongoOptions.FindOneAndReplace().SetReturnDocument(mongoOptions.After)))
	switch {
	case err == nil:
		return out, nil
	case !errors.Is(err, mongo.ErrNoDocuments):
		return retryqueue.Item[P]{}, coreerrs.WrapOperation(err, "reopen retry item")
	}
	// Nothing to move: the item is newer or already pending at item's
	// generation, or missing (Mark inserts it, or returns the current one).
	return s.Mark(ctx, item)
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
		bson.M{"$set": bson.M{fNextAttempt: now.UTC()}},
	)
	if err != nil {
		return coreerrs.WrapOperation(err, "wake due retry items")
	}
	return nil
}

// Reschedule implements [retryqueue.Storage].
func (s *Store[P]) Reschedule(ctx context.Context, key string, generation int64, next time.Time) error {
	_, err := s.coll.UpdateOne(ctx,
		bson.M{fID: key, fState: string(retryqueue.StatePending), fGeneration: genEq(generation)},
		bson.M{"$inc": bson.M{fAttempts: 1}, "$set": bson.M{fNextAttempt: next.UTC()}},
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
		bson.M{"$set": bson.M{fState: string(retryqueue.StateExpired), fExpiredAt: at.UTC()}},
	)
	if err != nil {
		return false, coreerrs.WrapOperation(err, "expire retry item")
	}
	return res.ModifiedCount > 0, nil
}

// Resolve implements [retryqueue.Storage].
func (s *Store[P]) Resolve(ctx context.Context, key string, generation int64, at time.Time) error {
	_, err := s.coll.UpdateOne(ctx,
		bson.M{fID: key, fState: bson.M{"$ne": string(retryqueue.StateResolved)}, fGeneration: genEq(generation)},
		bson.M{
			"$set":   bson.M{fState: string(retryqueue.StateResolved), fResolvedAt: at.UTC()},
			"$unset": bson.M{fExpiredAt: "", s.flagField: "", s.reasonField: ""},
		},
	)
	if err != nil {
		return coreerrs.WrapOperation(err, "resolve retry item")
	}
	return nil
}

// Flag implements [retryqueue.Storage].
func (s *Store[P]) Flag(ctx context.Context, key string, generation int64, reason string) (bool, error) {
	res, err := s.coll.UpdateOne(ctx,
		bson.M{fID: key, fState: string(retryqueue.StatePending), fGeneration: genEq(generation), "$or": bson.A{
			bson.M{s.flagField: bson.M{"$ne": true}},
			bson.M{s.reasonField: bson.M{"$ne": reason}},
		}},
		bson.M{"$set": bson.M{s.flagField: true, s.reasonField: reason}},
	)
	if err != nil {
		return false, coreerrs.WrapOperation(err, "flag retry item")
	}
	return res.ModifiedCount > 0, nil
}

// ClearFlag implements [retryqueue.Storage].
func (s *Store[P]) ClearFlag(ctx context.Context, key string, generation int64) error {
	_, err := s.coll.UpdateOne(ctx,
		bson.M{fID: key, fGeneration: genEq(generation), s.flagField: true},
		bson.M{"$unset": bson.M{s.flagField: "", s.reasonField: ""}},
	)
	if err != nil {
		return coreerrs.WrapOperation(err, "clear retry item flag")
	}
	return nil
}

// DeleteObsolete implements [retryqueue.Storage]: a conditional delete, then,
// when nothing was deleted, a look-up of a newer item.
func (s *Store[P]) DeleteObsolete(ctx context.Context, key string, generation int64) (bool, error) {
	res, err := s.coll.DeleteOne(ctx, bson.M{fID: key, "$or": bson.A{
		bson.M{fGeneration: bson.M{"$lte": generation}},
		bson.M{fGeneration: bson.M{"$exists": false}},
	}})
	if err != nil {
		return false, coreerrs.WrapOperation(err, "delete obsolete retry item")
	}
	if res.DeletedCount > 0 {
		return false, nil
	}
	err = s.coll.FindOne(ctx, bson.M{fID: key, fGeneration: bson.M{"$gt": generation}},
		mongoOptions.FindOne().SetProjection(bson.M{fID: 1})).Err()
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return false, nil
	case err != nil:
		return false, coreerrs.WrapOperation(err, "look up newer retry item")
	}
	return true, nil
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
	res, err := s.coll.UpdateMany(ctx, filter, bson.M{
		"$set": bson.M{
			fState: string(retryqueue.StatePending), fSince: now.UTC(), fNextAttempt: now.UTC(), fAttempts: 0,
		},
		"$unset": bson.M{fExpiredAt: ""},
	})
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "requeue retry items")
	}
	return int(res.ModifiedCount), nil
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

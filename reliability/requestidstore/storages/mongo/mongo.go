// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-aip/requestid"

	"github.com/altessa-s/go-atlas/store/mongo/indexes"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	datamongo "github.com/altessa-s/go-atlas/store/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const fieldID = "_id"

// Storage persists AIP-155 request IDs in a MongoDB collection.
type Storage struct {
	collection *mongo.Collection
}

var _ requestid.Store = (*Storage)(nil)

// New opens a dedicated collection and ensures its garbage-collection index.
func New(ctx context.Context, db *mongo.Database, opts ...Option) (*Storage, error) {
	if db == nil {
		return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "MongoDB database is required")
	}
	coll := db.Collection(newOptions(opts...).collectionName)
	if err := indexes.Ensure(ctx, coll, []mongo.IndexModel{{
		Keys:    bson.D{{Key: "purge_at", Value: 1}},
		Options: mongoopts.Index().SetExpireAfterSeconds(0),
	}}); err != nil {
		return nil, coreerrs.WrapOperation(err, "ensure request ID TTL index")
	}
	return &Storage{collection: coll}, nil
}

// record keeps the token as BSON binary so even a collection with a non-simple
// collation compares ownership byte for byte.
type record struct {
	State           requestid.State `bson:"state"`
	Token           []byte          `bson:"token"`
	Fingerprint     []byte          `bson:"fingerprint"`
	Response        []byte          `bson:"response"`
	Failure         []byte          `bson:"failure"`
	ResponseOmitted bool            `bson:"response_omitted"`
	ExpiresAt       time.Time       `bson:"expires_at"`
}

func stored(e requestid.Entry) record {
	return record{State: e.State, Token: []byte(e.Token), Fingerprint: e.Fingerprint,
		Response: e.Response, Failure: e.Failure, ResponseOmitted: e.ResponseOmitted, ExpiresAt: e.ExpiresAt}
}

func (r record) entry() requestid.Entry {
	return requestid.Entry{State: r.State, Token: string(r.Token), Fingerprint: r.Fingerprint,
		Response: r.Response, Failure: r.Failure, ResponseOmitted: r.ResponseOmitted, ExpiresAt: r.ExpiresAt}
}

// replacement uses the server clock only to schedule physical deletion. The
// TTL monitor sees a full relative retention period even if the caller's clock
// is behind the MongoDB server. The margin absorbs small changes in skew.
func replacement(key string, entry requestid.Entry, now time.Time) bson.M {
	const grace = time.Minute
	retainMillis := max(int64(0), entry.ExpiresAt.UnixMilli()-now.UnixMilli()) + grace.Milliseconds()
	return bson.M{
		fieldID:    key,
		"entry":    bson.M{"$literal": stored(entry)},
		"purge_at": bson.M{"$add": bson.A{"$$NOW", retainMillis}},
	}
}

// Reserve atomically inserts or replaces an expired document. The pre-image
// distinguishes the winner even when two callers submit identical entries.
func (s *Storage) Reserve(ctx context.Context, key string, pending requestid.Entry, now time.Time) (requestid.Entry, bool, error) {
	if err := ctx.Err(); err != nil {
		return requestid.Entry{}, false, err
	}
	const expiry = "$entry.expires_at"
	available := bson.M{"$or": bson.A{
		bson.M{"$eq": bson.A{bson.M{"$type": expiry}, "missing"}},
		bson.M{"$lte": bson.A{expiry, now}},
	}}
	update := mongo.Pipeline{{{Key: "$replaceWith", Value: bson.M{
		"$cond": bson.A{available, replacement(key, pending, now), "$$ROOT"},
	}}}}
	filter := bson.M{fieldID: key}
	opts := mongoopts.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(mongoopts.Before)
	var old struct {
		Entry record `bson:"entry"`
	}
	err := s.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&old)
	if duplicate, _ := datamongo.IsErrorDuplicate(err); duplicate && !datamongo.InTransaction(ctx) {
		// A competing upsert created the document after the initial match.
		err = s.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&old)
	}
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		// Upsert inserted a new document.
	case err != nil:
		return requestid.Entry{}, false, coreerrs.WrapOperation(err, "reserve MongoDB request ID")
	case old.Entry.ExpiresAt.After(now):
		return old.Entry.entry(), false, nil
	}

	pending.Fingerprint = bytes.Clone(pending.Fingerprint)
	pending.Response = bytes.Clone(pending.Response)
	pending.Failure = bytes.Clone(pending.Failure)
	pending.ExpiresAt = pending.ExpiresAt.Truncate(time.Millisecond)
	return pending, true, nil
}

func owned(key, token string, now time.Time) bson.M {
	return bson.M{fieldID: key, "entry.state": requestid.StatePending,
		"entry.token": []byte(token), "entry.expires_at": bson.M{"$gt": now}}
}

// Complete replaces a live reservation owned by token with the completed record.
func (s *Storage) Complete(ctx context.Context, key, token string, done requestid.Entry, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	update := mongo.Pipeline{{{Key: "$replaceWith", Value: replacement(key, done, now)}}}
	opts := mongoopts.FindOneAndUpdate().SetProjection(bson.M{fieldID: 1})
	err := s.collection.FindOneAndUpdate(ctx, owned(key, token, now), update, opts).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return requestid.ErrReservationLost
	}
	return coreerrs.WrapOperation(err, "complete MongoDB request ID")
}

// Release removes a live reservation owned by token.
func (s *Storage) Release(ctx context.Context, key, token string, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	res, err := s.collection.DeleteOne(ctx, owned(key, token, now))
	if err != nil {
		return coreerrs.WrapOperation(err, "release MongoDB request ID")
	}
	if res.DeletedCount == 0 {
		return requestid.ErrReservationLost
	}
	return nil
}

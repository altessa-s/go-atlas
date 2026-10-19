// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/mongo/bsoncodec"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
)

// documentRegistry is the registry [SetOnInsertFrom] encodes with: the one [Mongo.Connect] installs on the clients it creates.
var documentRegistry = sync.OnceValue(bsoncodec.NewRegistry)

// SetOnInsertFrom returns the "$setOnInsert" document of an upsert keyed on filterKeys: every top-level field of doc except the
// filter's equality fields, which the upsert copies from the filter. doc is encoded once, with the go-atlas codec registry that
// [Mongo.Connect] installs and the default encoder settings, so struct tags (names, omitempty, inline) and the decimal and date codecs
// apply exactly as for an InsertOne of the same doc; the fields keep doc's order and are returned as encoded bson.RawValue, which any
// registry writes unchanged. A filter key doc does not produce (an omitted zero value) is ignored. doc must encode as a document.
//
//	onInsert, err := datamongo.SetOnInsertFrom(rateDoc, "source", "base", "quote", "effective_date", "version")
//	_, err = coll.UpdateOne(ctx, keyFilter, bson.D{{Key: "$setOnInsert", Value: onInsert}}, options.UpdateOne().SetUpsert(true))
func SetOnInsertFrom(doc any, filterKeys ...string) (bson.D, error) {
	var buf bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&buf))
	enc.SetRegistry(documentRegistry())
	if err := enc.Encode(doc); err != nil {
		return nil, coreerrs.WrapOperation(err, "encode $setOnInsert document")
	}
	elems, err := bson.Raw(buf.Bytes()).Elements()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read $setOnInsert document")
	}
	out := make(bson.D, 0, len(elems))
	for _, e := range elems {
		if slices.Contains(filterKeys, e.Key()) {
			continue
		}
		out = append(out, bson.E{Key: e.Key(), Value: e.Value()})
	}
	return out, nil
}

// UpsertRetryingDuplicate runs upsert up to attempts times (at least once), repeating it at once only when it fails with a
// duplicate-key error: two concurrent first upserts of one document race on a unique key and the loser's repeat then matches the
// document the winner inserted. Any other error, or the last duplicate, is returned as is. Inside a transaction ([InTransaction]) upsert
// runs exactly once: a failed write aborts the transaction, so only a retry of the whole transaction can recover. A canceled ctx between
// attempts returns ctx.Err().
func UpsertRetryingDuplicate(ctx context.Context, attempts int, upsert func(context.Context) error) error {
	if InTransaction(ctx) {
		attempts = 1
	}
	return coreretry.Do(ctx, upsert,
		coreretry.WithMaxAttempts(max(attempts, 1)-1), // the 0-based index of the last attempt
		coreretry.WithNextDelay(func(int, error) time.Duration { return time.Nanosecond }),
		coreretry.WithShouldRetry(mongo.IsDuplicateKeyError),
	)
}

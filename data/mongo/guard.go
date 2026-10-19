// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ErrGuardFieldRequired is returned by [TouchGuard] for an empty guard field.
var ErrGuardFieldRequired = errors.New("mongo: guard field is required")

// guardIncrement is the default $inc step of [TouchGuard]: an int32 1, the encoding of a Go int literal, so a guard field that other
// writers also bump with "$inc": 1 keeps one BSON type (MongoDB promotes it to int64 on overflow, and an int64 field stays int64).
const guardIncrement int32 = 1

// TouchGuard writes the document coll matches with filter by incrementing its counter field, and reports whether a document was
// touched. Run inside a transaction it serializes conflicting writers through that document: a concurrent transaction that writes the
// same document (another TouchGuard, a delete, any update) gets a write conflict, so exactly one of them commits and the driver
// retries the other. Typical guards are the referenced document of a reference check (a delete of the target then conflicts with the
// referrer's transaction) and a per-key lock document.
//
// Without upsert, a filter that matches nothing touches nothing and returns false with no error: the caller decides what a missing
// guard means (an unknown reference). With upsert, a missing guard document is inserted with the filter's equality fields and the
// counter at 1, and touched is true; two first upserts of one guard racing outside a transaction can fail with a duplicate-key error,
// returned wrapped (classify it with [IsErrorDuplicate]). Outside a transaction TouchGuard is a plain single-document write and
// serializes nothing beyond it.
//
// The counter is storage-only: its value carries no meaning and must not be read by the application. It is incremented by an int32
// 1, so a guard inserted by an upsert stores an int32; [WithInt64Increment] uses an int64 1 instead, for guard fields kept as int64
// (an existing int64 counter stays int64 either way).
func TouchGuard(ctx context.Context, coll *mongo.Collection, filter any, field string, upsert bool, opts ...GuardOption) (bool, error) {
	if field == "" {
		return false, ErrGuardFieldRequired
	}
	var inc any = guardIncrement
	if newGuardOptions(opts...).int64Increment {
		inc = int64(guardIncrement)
	}
	update := mongoOptions.UpdateOne()
	if upsert {
		update.SetUpsert(true)
	}
	res, err := coll.UpdateOne(ctx, filter, bson.D{{Key: "$inc", Value: bson.D{{Key: field, Value: inc}}}}, update)
	if err != nil {
		return false, coreerrs.WrapOperationWithContext(err, "touch guard", coll.Name()+"."+field)
	}
	return res.MatchedCount > 0 || res.UpsertedCount > 0, nil
}

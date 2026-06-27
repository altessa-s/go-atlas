// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// millisPerSecond converts between Unix seconds (the legacy int64 representation)
// and the millisecond epoch used by BSON Date: $toDate reads a number as
// milliseconds, and $toLong on a Date yields milliseconds.
const millisPerSecond = int64(1000) //nolint:mnd // definition of the named constant

// MigrateTimestampsToDate converts legacy outbox documents whose timestamp fields
// were stored as int64 Unix seconds into BSON Date, in place. It must be run once
// per collection before serving traffic with the current store, which compares
// these fields against the MongoDB server clock ($$NOW) and therefore requires
// them to be Date-typed.
//
// The conversion is idempotent and safe to re-run: only documents whose created_at
// is still numeric are touched (already-migrated documents store created_at as a
// Date and are skipped). Optional timestamps stored as 0 or absent (published_at,
// last_attempt_on, locked_on, expires_at) are removed rather than mapped to the
// Unix epoch, matching the pointer/omitempty representation of new documents.
//
// It returns the number of documents converted. Wire it into the deployment's
// migration registry (e.g. mongo-migrate) rather than calling it from New.
//
// Example:
//
//	n, err := outboxstore.MigrateTimestampsToDate(ctx, db.Collection("outbox_events"))
func MigrateTimestampsToDate(ctx context.Context, collection *mongo.Collection) (int64, error) {
	// secondsToDate maps a positive Unix-seconds number to a Date and drops a
	// zero/absent value. In an aggregation expression a missing field resolves to
	// null, so $gt:[null,0] is false and the field is removed via $$REMOVE.
	secondsToDate := func(field string) bson.M {
		path := "$" + field
		return bson.M{"$cond": bson.A{
			bson.M{"$gt": bson.A{path, 0}},
			bson.M{"$toDate": bson.M{"$multiply": bson.A{path, millisPerSecond}}},
			"$$REMOVE",
		}}
	}

	createdPath := "$" + collectionFieldCreatedAt
	pipeline := mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		// created_at is always present and positive in legacy documents.
		collectionFieldCreatedAt:     bson.M{"$toDate": bson.M{"$multiply": bson.A{createdPath, millisPerSecond}}},
		collectionFieldPublishedAt:   secondsToDate(collectionFieldPublishedAt),
		collectionFieldLastAttemptOn: secondsToDate(collectionFieldLastAttemptOn),
		collectionFieldLockedOn:      secondsToDate(collectionFieldLockedOn),
		collectionFieldExpiresAt:     secondsToDate(collectionFieldExpiresAt),
	}}}}

	// Only legacy documents (numeric created_at) are matched; Date-typed documents
	// are already migrated, which makes the operation idempotent.
	filter := bson.M{collectionFieldCreatedAt: bson.M{"$type": "number"}}

	res, err := collection.UpdateMany(ctx, filter, pipeline)
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "migrate outbox timestamps to BSON Date")
	}
	return res.ModifiedCount, nil
}

// RevertTimestampsToUnix is the inverse of MigrateTimestampsToDate: it converts
// Date-typed timestamp fields back to int64 Unix seconds, restoring the legacy
// layout so an older store build can read the collection. It exists so a
// deployment can roll back the BSON Date migration.
//
// Like the forward migration it is idempotent (only documents whose created_at is
// still a Date are touched) and it removes optional timestamps that are absent,
// which the legacy decoder reads back as zero.
//
// It returns the number of documents reverted.
func RevertTimestampsToUnix(ctx context.Context, collection *mongo.Collection) (int64, error) {
	// dateToSeconds maps a Date field to int64 Unix seconds and drops a
	// missing/non-date value (legacy decode reads an absent int64 as 0).
	dateToSeconds := func(field string) bson.M {
		path := "$" + field
		return bson.M{"$cond": bson.A{
			bson.M{"$eq": bson.A{bson.M{"$type": path}, "date"}},
			bson.M{"$toLong": bson.M{"$divide": bson.A{bson.M{"$toLong": path}, millisPerSecond}}},
			"$$REMOVE",
		}}
	}

	createdPath := "$" + collectionFieldCreatedAt
	pipeline := mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{
		collectionFieldCreatedAt:     bson.M{"$toLong": bson.M{"$divide": bson.A{bson.M{"$toLong": createdPath}, millisPerSecond}}},
		collectionFieldPublishedAt:   dateToSeconds(collectionFieldPublishedAt),
		collectionFieldLastAttemptOn: dateToSeconds(collectionFieldLastAttemptOn),
		collectionFieldLockedOn:      dateToSeconds(collectionFieldLockedOn),
		collectionFieldExpiresAt:     dateToSeconds(collectionFieldExpiresAt),
	}}}}

	filter := bson.M{collectionFieldCreatedAt: bson.M{"$type": "date"}}

	res, err := collection.UpdateMany(ctx, filter, pipeline)
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "revert outbox timestamps to Unix seconds")
	}
	return res.ModifiedCount, nil
}

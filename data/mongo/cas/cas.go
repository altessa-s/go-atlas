// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cas

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	// ErrNotFound is what [ClassifyMiss] returns for an absent document when the caller passes no not-found error. It wraps
	// [coreerrs.ErrNotFound].
	ErrNotFound = fmt.Errorf("cas: document %w", coreerrs.ErrNotFound)
	// ErrConflict is what [ClassifyMiss] returns for a document whose version moved on when the caller passes no conflict error.
	// It wraps [coreerrs.ErrConflict].
	ErrConflict = fmt.Errorf("cas: %w", coreerrs.ErrConflict)
)

// InitialVersion is the version of a newly inserted document: the insert time in nanoseconds instead of 1. A document that is deleted
// physically and inserted again under the same ID must not restart at a version its previous incarnation had, or a stale etag read
// before the delete would match the new document (ABA). Writes add 1, so a later incarnation starts above every version of an earlier
// one as long as the wall clock does not step back across the two inserts.
func InitialVersion() int64 { return time.Now().UnixNano() }

// Filter selects document id (stored under idField) at exactly the expected version stored in versionField:
// {idField: id, versionField: expected}. A caller may add further conditions to the returned map.
func Filter(idField string, id any, versionField string, expected int64) bson.M {
	return bson.M{idField: id, versionField: expected}
}

// Inc is the update document that bumps versionField by one atomically: {"$inc": {versionField: int64(1)}}. Add the data operators
// to the returned map so the change and the bump are a single write:
//
//	update := cas.Inc("version")
//	update["$set"] = fields
func Inc(versionField string) bson.M {
	return bson.M{"$inc": bson.M{versionField: int64(1)}}
}

// ClassifyMiss explains a CAS write that matched no document: it returns notFound when no document with idField = id exists, and
// conflict when one does (its version moved on). A nil notFound or conflict stands for [ErrNotFound] or [ErrConflict], so a miss never
// classifies as success. Pass the transaction's context inside a transaction so the check reads the same snapshot. A failed check is
// returned wrapped, never classified.
func ClassifyMiss(ctx context.Context, coll *mongo.Collection, idField string, id any, notFound, conflict error) error {
	n, err := coll.CountDocuments(ctx, bson.D{{Key: idField, Value: id}}, mongoOptions.Count().SetLimit(1))
	switch {
	case err != nil:
		return coreerrs.WrapOperationWithContext(err, "classify CAS miss", coll.Name())
	case n == 0:
		return cmp.Or(notFound, ErrNotFound)
	default:
		return cmp.Or(conflict, ErrConflict)
	}
}

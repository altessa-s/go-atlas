// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// newDetachedCollection returns a *mongo.Collection backed by a client that
// never dials: the driver connects lazily, and every test below fails inside
// ListCursor before any network operation is issued.
func newDetachedCollection(t *testing.T) *mongo.Collection {
	t.Helper()
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	require.NoError(t, err, "failed to create detached test client")
	return client.Database("testdb").Collection("items")
}

// collectSeqError drains the iterator and returns how many pairs were
// yielded along with the last error seen.
func collectSeqError(t *testing.T, coll *mongo.Collection, opts ...ListCursorOption) (int, error) {
	t.Helper()
	var (
		yielded int
		lastErr error
	)
	for _, err := range ListCursorSeq[bson.M](t.Context(), coll, opts...) {
		yielded++
		lastErr = err
	}
	return yielded, lastErr
}

func TestListCursorSeq_InvalidTokenYieldsSingleErrorAndStops(t *testing.T) {
	t.Parallel()
	coll := newDetachedCollection(t)

	yielded, err := collectSeqError(t, coll, WithListCursorCursor("%%%not-a-cursor%%%"))
	require.Equal(t, 1, yielded, "iterator must yield exactly one error pair and stop")
	require.ErrorIs(t, err, ErrInvalidCursor)
}

func TestListCursorSeq_ULIDTokenWithoutStorageFails(t *testing.T) {
	t.Parallel()
	coll := newDetachedCollection(t)

	yielded, err := collectSeqError(t, coll, WithListCursorCursor("01ARZ3NDEKTSV4RRFFQ69G5FAV"))
	require.Equal(t, 1, yielded)
	require.ErrorIs(t, err, ErrStorageRequired)
}

func TestListCursorSeq_FilterChangeMidPaginationFails(t *testing.T) {
	t.Parallel()
	coll := newDetachedCollection(t)

	cursor, err := NewCursorWithMetadata(
		bson.NewObjectID().Hex(),
		DefaultListSort,
		DefaultCursorIdField,
		bson.M{"status": "active"},
		nil,
	)
	require.NoError(t, err)

	yielded, err := collectSeqError(t, coll,
		WithListCursorCursor(cursor.String()),
		WithListCursorFilter(bson.M{"status": "archived"}),
	)
	require.Equal(t, 1, yielded)
	require.ErrorIs(t, err, ErrCursorFilterMismatch)
}

func TestListCursorSeq_SortChangeMidPaginationFails(t *testing.T) {
	t.Parallel()
	coll := newDetachedCollection(t)

	// Cursor minted with a non-default sort; the continuation request uses
	// the default sort, so the checksum validation must reject it.
	cursor, err := NewCursorWithMetadata(
		bson.NewObjectID().Hex(),
		bson.D{{Key: "name", Value: SortAscending}},
		DefaultCursorIdField,
		bson.M{},
		"last-name",
	)
	require.NoError(t, err)

	yielded, err := collectSeqError(t, coll, WithListCursorCursor(cursor.String()))
	require.Equal(t, 1, yielded)
	require.ErrorIs(t, err, ErrCursorChecksumMismatch)
}

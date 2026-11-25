// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestStatefulCursor_SubjectMatchIsExact pins that the subject of the request
// and of the stored cursor must be equal: a cursor minted without a subject
// (by an unscoped listing over the same collection, or before the endpoint
// bound its cursors) is not accepted by a request that passes one, and a bound
// cursor is not accepted by a request that passes none.
func TestStatefulCursor_SubjectMatchIsExact(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	sort := bson.D{{Key: "name", Value: 1}}
	filter := bson.M{"status": "active"}

	tests := []struct {
		name      string
		minted    string
		requested string
	}{
		{name: "unbound cursor under a subject", minted: "", requested: "tenant-A"},
		{name: "bound cursor without a subject", minted: "tenant-A", requested: ""},
		{name: "bound cursor under another subject", minted: "tenant-A", requested: "tenant-B"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storage := newFakeCursorStorage()
			token, err := storeCursor(t.Context(), bson.NewObjectID().Hex(), sort, DefaultCursorIdField, cursorFilter{original: filter}, storage,
				"alice", tc.minted, cursorCollectionDigest(coll))
			require.NoError(t, err)

			_, err = ListCursor[bson.M](shortCtx(t), coll,
				WithListCursorStorage(storage),
				WithListCursorSubject(tc.requested),
				WithListCursorFilter(filter),
				WithListCursorSort(sort),
				WithListCursorCursor(token),
			)
			require.ErrorIs(t, err, ErrCursorSubjectMismatch)
		})
	}
}

// TestStatefulCursor_SameSubjectResolves is the counterpart: an equal subject,
// empty or not, resolves the stored cursor.
func TestStatefulCursor_SameSubjectResolves(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	sort := bson.D{{Key: "name", Value: 1}}
	filter := bson.M{"status": "active"}

	for _, subject := range []string{"", "tenant-A"} {
		t.Run("subject="+subject, func(t *testing.T) {
			t.Parallel()
			storage := newFakeCursorStorage()
			token, err := storeCursor(t.Context(), bson.NewObjectID().Hex(), sort, DefaultCursorIdField, cursorFilter{original: filter}, storage,
				"alice", subject, cursorCollectionDigest(coll))
			require.NoError(t, err)

			cursor, err := loadStoredCursor(t.Context(), token, storage, cursorFilter{original: filter}, subject, cursorCollectionDigest(coll))
			require.NoError(t, err)
			require.NotNil(t, cursor)
		})
	}
}

// TestStatefulCursor_RecordWithoutCollection pins the handling of a record
// stored before the collection binding existed: it decodes, and is rejected
// on every collection, since nothing tells which endpoint issued it.
func TestStatefulCursor_RecordWithoutCollection(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	sort := bson.D{{Key: "name", Value: 1}}
	filter := bson.M{"status": "active"}
	const key = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	current, err := NewCursorMetadata(bson.NewObjectID().Hex(), sort, DefaultCursorIdField, filter, "alice")
	require.NoError(t, err)
	encoded, err := json.Marshal(current)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"collection"`, "an unbound record is stored in the previous format")

	var legacy CursorMetadata
	require.NoError(t, json.Unmarshal(encoded, &legacy))
	require.Empty(t, legacy.Collection)
	storage := newFakeCursorStorage()
	require.NoError(t, storage.Store(t.Context(), key, &legacy))

	_, err = ListCursor[bson.M](shortCtx(t), coll,
		WithListCursorStorage(storage),
		WithListCursorFilter(filter),
		WithListCursorSort(sort),
		WithListCursorCursor(key),
	)
	require.ErrorIs(t, err, ErrCursorCollectionMismatch)
}

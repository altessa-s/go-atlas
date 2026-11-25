// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// mintStoredCursor stores a "next" cursor the way ListCursor does for a page
// of collection and returns its storage key.
func mintStoredCursor(t testing.TB, storage Storage, collection *mongo.Collection, sort bson.D, filter bson.M) string {
	t.Helper()
	token, err := storeCursor(t.Context(), bson.NewObjectID().Hex(), sort, DefaultCursorIdField, cursorFilter{original: filter}, storage,
		"alice", "tenant-A", cursorCollectionDigest(collection))
	require.NoError(t, err)
	return token
}

// TestStatefulCursor_CrossCollectionReplayRejected pins that a token issued by
// one endpoint is not accepted by another one sharing the cursor storage, even
// when filter, sort and subject are the same.
func TestStatefulCursor_CrossCollectionReplayRejected(t *testing.T) {
	t.Parallel()

	issuer := newDetachedCollection(t)
	sort := bson.D{{Key: "name", Value: 1}}
	filter := bson.M{"status": "active"}

	tests := []struct {
		name   string
		target *mongo.Collection
	}{
		{name: "another collection", target: issuer.Database().Collection("other")},
		{name: "same name in another database", target: issuer.Database().Client().Database("otherdb").Collection(issuer.Name())},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storage := newFakeCursorStorage()
			token := mintStoredCursor(t, storage, issuer, sort, filter)

			_, err := ListCursor[bson.M](shortCtx(t), tc.target,
				WithListCursorStorage(storage),
				WithListCursorSubject("tenant-A"),
				WithListCursorFilter(filter),
				WithListCursorSort(sort),
				WithListCursorCursor(token),
			)
			require.ErrorIs(t, err, ErrCursorCollectionMismatch)
		})
	}
}

// TestStatefulCursor_ChangedPaginationParametersRejected pins that a stored
// cursor does not force its sort or cursor ID field onto a request that asks
// for other ones. An omitted sort is the default sort, as in stateless mode.
func TestStatefulCursor_ChangedPaginationParametersRejected(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	sort := bson.D{{Key: "name", Value: 1}}
	filter := bson.M{"status": "active"}

	tests := []struct {
		name string
		opts []ListCursorOption
	}{
		{name: "another sort field", opts: []ListCursorOption{WithListCursorSort("email")}},
		{name: "another direction", opts: []ListCursorOption{WithListCursorSort("-name")}},
		{name: "omitted sort", opts: nil},
		{name: "another cursor ID field", opts: []ListCursorOption{WithListCursorSort(sort), WithListCursorIdField("_id")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storage := newFakeCursorStorage()
			token := mintStoredCursor(t, storage, coll, sort, filter)

			opts := append([]ListCursorOption{
				WithListCursorStorage(storage),
				WithListCursorSubject("tenant-A"),
				WithListCursorFilter(filter),
				WithListCursorCursor(token),
			}, tc.opts...)
			_, err := ListCursor[bson.M](shortCtx(t), coll, opts...)
			require.ErrorIs(t, err, ErrCursorChecksumMismatch)
		})
	}
}

// TestStatefulCursor_ContinuationWithSameParameters pins the contract: a
// continuation that repeats the parameters of the first page resolves the
// stored cursor, with an explicit sort and with the default one alike.
func TestStatefulCursor_ContinuationWithSameParameters(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	filter := bson.M{"status": "active"}

	tests := []struct {
		name string
		sort bson.D
		opts []ListCursorOption
	}{
		{name: "explicit sort", sort: bson.D{{Key: "name", Value: 1}}, opts: []ListCursorOption{WithListCursorSort("name")}},
		{name: "default sort", sort: DefaultListSort, opts: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storage := newFakeCursorStorage()
			token := mintStoredCursor(t, storage, coll, tc.sort, filter)

			opts := newListCursorOptions(append([]ListCursorOption{
				WithListCursorStorage(storage),
				WithListCursorSubject("tenant-A"),
				WithListCursorFilter(filter),
				WithListCursorCursor(token),
			}, tc.opts...)...)
			require.NoError(t, resolveCursorOptions(t.Context(), opts, cursorCollectionDigest(coll)))
			require.NotNil(t, opts.cursor)
			// The stored sort is decoded BSON (int32 directions): compare encodings.
			want, err := encodeSortToString(tc.sort)
			require.NoError(t, err)
			got, err := encodeSortToString(opts.sort)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

// TestCursorCollectionDigest_Unambiguous pins that the digest separates the
// database from the collection name instead of joining them.
func TestCursorCollectionDigest_Unambiguous(t *testing.T) {
	t.Parallel()

	client := newDetachedCollection(t).Database().Client()
	first := cursorCollectionDigest(client.Database("a").Collection("b.c"))
	second := cursorCollectionDigest(client.Database("a.b").Collection("c"))

	require.NotEqual(t, first, second)
	require.Equal(t, first, cursorCollectionDigest(client.Database("a").Collection("b.c")))
}

// TestCursorFilterMismatch_HidesFilterHashes pins that the mismatch error,
// which a service may pass on to the client, does not spell the hash of the
// server-side filter or of the requested one.
func TestCursorFilterMismatch_HidesFilterHashes(t *testing.T) {
	t.Parallel()

	sort := bson.D{{Key: "name", Value: 1}}
	issued := bson.M{"tenant": "A"}
	requested := bson.M{"tenant": "B"}
	issuedHash, err := computeFilterHash(issued)
	require.NoError(t, err)
	requestedHash, err := computeFilterHash(requested)
	require.NoError(t, err)

	cursor, err := NewCursorWithMetadata(bson.NewObjectID().Hex(), sort, DefaultCursorIdField, issued, "alice")
	require.NoError(t, err)
	metadata, err := NewCursorMetadata(bson.NewObjectID().Hex(), sort, DefaultCursorIdField, issued, "alice")
	require.NoError(t, err)

	for name, err := range map[string]error{
		"stateless": cursor.ValidateFilter(requested),
		"stateful":  metadata.ValidateFilter(requested),
	} {
		require.ErrorIs(t, err, ErrCursorFilterMismatch, name)
		require.NotContains(t, err.Error(), issuedHash, name)
		require.NotContains(t, err.Error(), requestedHash, name)
	}
}

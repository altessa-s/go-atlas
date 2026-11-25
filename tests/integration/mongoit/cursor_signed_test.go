// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
)

// cursorSigner returns a page-token codec over a fixed test key.
func cursorSigner(t testing.TB) *pagetoken.Codec {
	t.Helper()
	codec, err := pagetoken.New(bytes.Repeat([]byte{7}, pagetoken.MinKeyLength))
	require.NoError(t, err)
	return codec
}

// A signed stateless cursor pages a live collection and is held to the
// subject and the collection it was issued for, with nothing stored
// server-side; an unsigned token is not accepted in its place, and a subject
// without a signer or a storage is refused.
func TestListCursor_SignedStateless(t *testing.T) {
	t.Parallel()

	coll := keysetColl(t)
	docs := make([]any, 0, 6)
	for i := range 6 {
		docs = append(docs, bson.M{"_id": bson.NewObjectID(), "created_at": bson.DateTime(i), "tenant": "A"})
	}
	_, err := coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)

	base := []datamongo.ListCursorOption{
		datamongo.WithListCursorIdField("_id"),
		datamongo.WithListCursorSort(bson.D{{Key: "created_at", Value: 1}}),
		datamongo.WithListCursorFilter(bson.M{"tenant": "A"}),
		datamongo.WithListCursorLimit(2),
	}
	list := func(opts ...datamongo.ListCursorOption) (*datamongo.ListCursorResult[bson.M], error) {
		return datamongo.ListCursor[bson.M](t.Context(), coll, append(base[:len(base):len(base)], opts...)...)
	}
	signer := datamongo.WithListCursorSigner(cursorSigner(t))
	tenant := datamongo.WithListCursorSubject("tenant-A")

	_, err = list(tenant)
	require.ErrorIs(t, err, datamongo.ErrCursorSubjectNotEnforced, "a subject without a signer or a storage")

	first, err := list(signer, tenant)
	require.NoError(t, err)
	require.Len(t, first.Items, 2)
	require.NotNil(t, first.NextCursor)
	token := datamongo.WithListCursorCursor(first.NextCursor)

	seen := map[any]bool{first.Items[0]["_id"]: true, first.Items[1]["_id"]: true}
	for next := first.NextCursor; next != nil; {
		page, pageErr := list(signer, tenant, datamongo.WithListCursorCursor(next))
		require.NoError(t, pageErr, "a signed token continues its own listing")
		for _, item := range page.Items {
			require.False(t, seen[item["_id"]], "duplicate item")
			seen[item["_id"]] = true
		}
		next = page.NextCursor
	}
	require.Len(t, seen, len(docs))

	tampered := []byte(*first.NextCursor)
	tampered[len(tampered)/2] ^= 1
	_, err = list(signer, tenant, datamongo.WithListCursorCursor(string(tampered)))
	require.ErrorIs(t, err, datamongo.ErrInvalidCursor, "a tampered token")

	_, err = list(signer, datamongo.WithListCursorSubject("tenant-B"), token)
	require.ErrorIs(t, err, datamongo.ErrCursorSubjectMismatch, "another subject")

	_, err = list(signer, token)
	require.ErrorIs(t, err, datamongo.ErrCursorSubjectMismatch, "a bound token without a subject")

	_, err = datamongo.ListCursor[bson.M](t.Context(), coll.Database().Collection("other"),
		append(base[:len(base):len(base)], signer, tenant, token)...)
	require.ErrorIs(t, err, datamongo.ErrCursorCollectionMismatch, "another collection")

	unsigned, err := list()
	require.NoError(t, err)
	require.NotNil(t, unsigned.NextCursor)
	_, err = list(signer, datamongo.WithListCursorCursor(unsigned.NextCursor))
	require.ErrorIs(t, err, datamongo.ErrInvalidCursor, "an unsigned token where a signer is configured")

	_, err = list(token)
	require.ErrorIs(t, err, datamongo.ErrInvalidCursor, "a signed token where no signer is configured")
}

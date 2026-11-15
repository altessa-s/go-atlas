// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
	cursorredis "github.com/altessa-s/go-atlas/store/mongo/storages/redis"
)

// A stored cursor continues only under the subject it was minted with. The
// unscoped listing and the tenant-scoped one page the same collection with the
// same filter and sort over one storage, so the subject is all that tells
// their tokens apart. The records go through Redis: one minted without a
// subject is stored without the field, like a record that predates the binding.
func TestListCursor_StatefulSubjectIsExact(t *testing.T) {
	t.Parallel()

	rdb := livebackend.ConnectRedis(t)
	storage := cursorredis.New(rdb, cursorredis.WithKeyPrefix(livebackend.RedisPrefix(t, rdb, "mongoit_cursor_subject")))

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
		datamongo.WithListCursorStorage(storage),
	}
	list := func(opts ...datamongo.ListCursorOption) (*datamongo.ListCursorResult[bson.M], error) {
		return datamongo.ListCursor[bson.M](t.Context(), coll, append(base[:len(base):len(base)], opts...)...)
	}
	tenant := datamongo.WithListCursorSubject("tenant-A")

	unscoped, err := list()
	require.NoError(t, err)
	require.NotNil(t, unscoped.NextCursor)
	scoped, err := list(tenant)
	require.NoError(t, err)
	require.NotNil(t, scoped.NextCursor)

	_, err = list(tenant, datamongo.WithListCursorCursor(unscoped.NextCursor))
	require.ErrorIs(t, err, datamongo.ErrCursorSubjectMismatch, "an unbound cursor under a subject")

	_, err = list(datamongo.WithListCursorCursor(scoped.NextCursor))
	require.ErrorIs(t, err, datamongo.ErrCursorSubjectMismatch, "a bound cursor without a subject")

	_, err = list(datamongo.WithListCursorSubject("tenant-B"), datamongo.WithListCursorCursor(scoped.NextCursor))
	require.ErrorIs(t, err, datamongo.ErrCursorSubjectMismatch, "a bound cursor under another subject")

	next, err := list(datamongo.WithListCursorCursor(unscoped.NextCursor))
	require.NoError(t, err, "the unscoped listing continues its own cursor")
	require.Len(t, next.Items, 2)

	next, err = list(tenant, datamongo.WithListCursorCursor(scoped.NextCursor))
	require.NoError(t, err, "the tenant continues its own cursor")
	require.Len(t, next.Items, 2)
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	cursredis "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/redis"
	mongohelpers "github.com/altessa-s/go-atlas/data/mongo/internal/testhelpers"
)

func setupStorage(tb testing.TB) *cursredis.Storage {
	tb.Helper()
	client, _ := testhelpers.RedisClient(tb)
	return cursredis.New(client)
}

func TestNew(t *testing.T) {
	s := setupStorage(t)
	require.NotNil(t, s)
}

func TestStorage_StoreLoad(t *testing.T) {
	s := setupStorage(t)
	ctx := t.Context()
	meta := mongohelpers.SampleCursorMetadata()

	require.NoError(t, s.Store(ctx, "key1", meta))

	loaded, err := s.Load(ctx, "key1")
	require.NoError(t, err)
	require.Equal(t, meta.CursorId, loaded.CursorId)
}

func TestStorage_Load_NotFound(t *testing.T) {
	s := setupStorage(t)

	_, err := s.Load(t.Context(), "missing")
	require.ErrorIs(t, err, mongo.ErrCursorNotFound)
}

func TestStorage_Delete(t *testing.T) {
	s := setupStorage(t)
	ctx := t.Context()

	_ = s.Store(ctx, "key1", mongohelpers.SampleCursorMetadata())
	require.NoError(t, s.Delete(ctx, "key1"))

	_, err := s.Load(ctx, "key1")
	require.ErrorIs(t, err, mongo.ErrCursorNotFound)
}

func TestStorage_Delete_NonExistent(t *testing.T) {
	s := setupStorage(t)
	require.NoError(t, s.Delete(t.Context(), "nope"))
}

func TestStorage_Overwrite(t *testing.T) {
	s := setupStorage(t)
	ctx := t.Context()

	meta1 := mongohelpers.SampleCursorMetadata()
	meta1.CursorId = "aaa"
	_ = s.Store(ctx, "key", meta1)

	meta2 := mongohelpers.SampleCursorMetadata()
	meta2.CursorId = "bbb"
	_ = s.Store(ctx, "key", meta2)

	loaded, _ := s.Load(ctx, "key")
	require.Equal(t, "bbb", loaded.CursorId)
}

func TestStorage_WithCustomOptions(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := cursredis.New(client,
		cursredis.WithTTL(30*time.Minute),
		cursredis.WithKeyPrefix("custom:"),
	)

	ctx := t.Context()
	_ = s.Store(ctx, "key1", mongohelpers.SampleCursorMetadata())

	loaded, err := s.Load(ctx, "key1")
	require.NoError(t, err)
	require.Equal(t, mongohelpers.SampleCursorMetadata().CursorId, loaded.CursorId)
}

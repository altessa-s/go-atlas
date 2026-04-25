// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisutils_test

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/redisutils"

	goredis "github.com/redis/go-redis/v9"
)

func setupRedis(tb testing.TB) (*goredis.Client, *miniredis.Miniredis) {
	tb.Helper()
	mr := miniredis.RunT(tb)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return client, mr
}

func TestGetBytes_Hit(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	mr.Set("key1", "value1")

	got, err := redisutils.GetBytes(ctx, client, "key1", errors.New("not found"))
	require.NoError(t, err)
	require.Equal(t, "value1", string(got))
}

func TestGetBytes_Miss(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	notFoundErr := errors.New("not found")
	_, err := redisutils.GetBytes(ctx, client, "missing", notFoundErr)
	require.ErrorIs(t, err, notFoundErr)
}

func TestSetBytes(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	err := redisutils.SetBytes(ctx, client, "key1", []byte("val"), 0)
	require.NoError(t, err)

	got, err := mr.Get("key1")
	require.NoError(t, err)
	require.Equal(t, "val", got)
}

func TestSetBytes_WithTTL(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	err := redisutils.SetBytes(ctx, client, "key1", []byte("val"), 10*time.Second)
	require.NoError(t, err)

	mr.FastForward(15 * time.Second)

	require.False(t, mr.Exists("key1"), "key should have expired")
}

func TestDel(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	mr.Set("key1", "value1")

	err := redisutils.Del(ctx, client, "key1")
	require.NoError(t, err)
	require.False(t, mr.Exists("key1"), "key should have been deleted")
}

func TestDel_NonExistent(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	err := redisutils.Del(ctx, client, "missing")
	require.NoError(t, err)
}

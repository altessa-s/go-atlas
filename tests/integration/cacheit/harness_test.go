// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cacheit_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/storages"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	cacheredis "github.com/altessa-s/go-atlas/data/cache/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

const (
	// settleWindow bounds the wait for a key to age out on the server.
	settleWindow = 15 * time.Second

	// samplingInterval is the poll cadence while waiting.
	samplingInterval = 50 * time.Millisecond

	// shortTTL is the lifetime of an entry a test waits to expire.
	shortTTL = time.Second
)

// fixture is a Redis cache storage over a key prefix of its own, with the
// raw client to read what the storage left on the server.
type fixture struct {
	storage *cacheredis.Storage
	client  *goredis.Client
	prefix  string
}

func newFixture(tb testing.TB) fixture {
	tb.Helper()
	client := livebackend.ConnectRedis(tb)
	prefix := livebackend.RedisPrefix(tb, client, "cacheit")
	return fixture{
		storage: cacheredis.New(client, cacheredis.WithPrefix(prefix)),
		client:  client,
		prefix:  prefix,
	}
}

// rawKey returns the server-side name of the storage key.
func (f fixture) rawKey(key string) string {
	return f.prefix + key
}

// pttl returns the remaining lifetime of the storage key on the server:
// -1 for a key without expiry, -2 for a missing key.
func (f fixture) pttl(tb testing.TB, key string) time.Duration {
	tb.Helper()
	d, err := f.client.PTTL(tb.Context(), f.rawKey(key)).Result()
	require.NoError(tb, err)
	return d
}

// awaitGone polls until the storage key is no longer on the server.
func (f fixture) awaitGone(tb testing.TB, key string) {
	tb.Helper()
	require.Eventually(tb, func() bool {
		n, err := f.client.Exists(context.Background(), f.rawKey(key)).Result()
		return err == nil && n == 0
	}, settleWindow, samplingInterval, "key %q never expired", key)
}

// countingStorage counts the misses of the storage it wraps, so a test can
// tell when every concurrent caller has looked the key up and found nothing.
type countingStorage struct {
	storages.Storage
	misses atomic.Int32
}

func (s *countingStorage) Get(ctx context.Context, key string) ([]byte, error) {
	v, err := s.Storage.Get(ctx, key)
	if errors.Is(err, storages.ErrMissing) {
		s.misses.Add(1)
	}
	return v, err
}

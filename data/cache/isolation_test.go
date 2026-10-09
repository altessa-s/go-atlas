// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache"
	"github.com/altessa-s/go-atlas/data/cache/storages/lru"
)

type namespaceKey struct{}

func TestNamespaceEncoding(t *testing.T) {
	t.Parallel()
	storage, err := lru.New(64)
	require.NoError(t, err)
	c := cache.New(storage, cache.WithKeyNamespace(func(ctx context.Context) string { return ctx.Value(namespaceKey{}).(string) }))
	pairs := []struct{ ns, key, value string }{
		{"org:team", "record", "a"}, {"org", "team:record", "b"}, {"", "org:team:record", "c"},
	}
	for _, pair := range pairs {
		ctx := context.WithValue(t.Context(), namespaceKey{}, pair.ns)
		require.NoError(t, c.Save(ctx, pair.key, pair.value))
	}
	for _, pair := range pairs {
		ctx := context.WithValue(t.Context(), namespaceKey{}, pair.ns)
		var value string
		require.NoError(t, c.Get(ctx, pair.key, &value))
		require.Equal(t, pair.value, value)
		require.NoError(t, c.DeleteMany(ctx, pair.key))
		_, err := cache.GetWithFallbackT(ctx, c, pair.key, func(context.Context) (string, time.Duration, error) {
			return pair.value, cache.TTLUseDefault, nil
		})
		require.NoError(t, err)
	}
}

func TestNilFallbackAndSharedDeadline(t *testing.T) {
	t.Parallel()
	c := cache.NewNoop(cache.WithFallbackTimeout(time.Millisecond))
	var value string
	err := c.GetWithFallback(t.Context(), "nil", &value, func(context.Context) (any, time.Duration, error) {
		return nil, cache.TTLUseDefault, nil
	})
	require.ErrorIs(t, err, cache.ErrMissing)
	err = c.GetWithFallback(t.Context(), "deadline", &value, func(ctx context.Context) (any, time.Duration, error) {
		<-ctx.Done()
		return nil, cache.TTLUseDefault, ctx.Err()
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = c.GetWithFallback(ctx, "canceled", &value, func(context.Context) (any, time.Duration, error) { panic("must not start") })
	require.ErrorIs(t, err, context.Canceled)
}

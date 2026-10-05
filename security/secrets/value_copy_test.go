// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/retry"
	"github.com/altessa-s/go-atlas/security/secrets"

	secretmemory "github.com/altessa-s/go-atlas/security/secrets/providers/memory"
)

// blockingProvider serves fixed payloads; its Value calls wait for release
// once armed, so concurrent fetches share one singleflight flight.
type blockingProvider[T any] struct {
	payload func() T
	release chan struct{}
	calls   atomic.Int32
}

func (p *blockingProvider[T]) Name() string { return "blocking" }

func (p *blockingProvider[T]) List(context.Context) ([]*secrets.Value[T], error) { return nil, nil }

func (p *blockingProvider[T]) Values(context.Context) iter.Seq2[*secrets.Value[T], error] {
	return func(func(*secrets.Value[T], error) bool) {}
}

func (p *blockingProvider[T]) Value(ctx context.Context, key string) (*secrets.Value[T], error) {
	p.calls.Add(1)
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return secrets.NewValue(key, p.payload(), nil, "1"), nil
}

func (p *blockingProvider[T]) Save(context.Context, string, T) error { return nil }
func (p *blockingProvider[T]) Delete(context.Context, string) error  { return nil }

// TestManager_Value_ReturnsCallerOwnedCopy checks that clearing a value
// returned by Value affects neither the cache nor other callers, on a cache
// hit and on a fetch shared by concurrent callers.
func TestManager_Value_ReturnsCallerOwnedCopy(t *testing.T) {
	t.Parallel()

	t.Run("cache hit", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		inner, err := secretmemory.New(map[string]string{"key-a": "value-a"})
		require.NoError(t, err)
		mgr, err := secrets.New[string](inner)
		require.NoError(t, err)
		_, err = mgr.Value(ctx, "key-a", true)
		require.NoError(t, err)

		first, err := mgr.Value(ctx, "key-a", false)
		require.NoError(t, err)
		second, err := mgr.Value(ctx, "key-a", false)
		require.NoError(t, err)
		require.NotSame(t, first, second)

		first.Clear()
		require.Equal(t, "value-a", second.Value)
		shared, err := mgr.ValueShared(ctx, "key-a", false)
		require.NoError(t, err)
		require.Equal(t, "value-a", shared.Value, "clearing a returned value must not clear the cache")
	})

	t.Run("shared fetch", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		provider := &blockingProvider[string]{payload: func() string { return string([]byte("value-a")) }, release: make(chan struct{})}
		mgr, err := secrets.New[string](provider)
		require.NoError(t, err)

		results := make(chan *secrets.Value[string], 2)
		for range 2 {
			go func() {
				v, err := mgr.Value(ctx, "key-a", true)
				if err != nil {
					results <- nil
					return
				}
				results <- v
			}()
		}
		time.Sleep(50 * time.Millisecond) // let both callers join the flight
		close(provider.release)
		a, b := <-results, <-results
		require.NotNil(t, a)
		require.NotNil(t, b)
		require.NotSame(t, a, b)

		a.Clear()
		require.Equal(t, "value-a", b.Value)
		cached, err := mgr.ValueShared(ctx, "key-a", false)
		require.NoError(t, err)
		require.Equal(t, "value-a", cached.Value)
	})
}

// TestManager_ValueShared_ReturnsCachedInstance checks the zero-copy
// accessor returns the Manager's cached instance on every hit.
func TestManager_ValueShared_ReturnsCachedInstance(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	inner, err := secretmemory.New(map[string]string{"key-a": "value-a"})
	require.NoError(t, err)
	mgr, err := secrets.New[string](inner)
	require.NoError(t, err)

	fetched, err := mgr.ValueShared(ctx, "key-a", true)
	require.NoError(t, err)
	require.Equal(t, "value-a", fetched.Value)

	first, err := mgr.ValueShared(ctx, "key-a", false)
	require.NoError(t, err)
	second, err := mgr.ValueShared(ctx, "key-a", false)
	require.NoError(t, err)
	require.Same(t, first, second)

	copied, err := mgr.Value(ctx, "key-a", false)
	require.NoError(t, err)
	require.NotSame(t, first, copied)
	require.Equal(t, first.Value, copied.Value)

	_, err = mgr.ValueShared(ctx, "missing", false)
	require.ErrorIs(t, err, secrets.ErrNotFound)
}

// TestManager_Value_MutationIsolation checks that mutating a returned value
// with reference payloads leaves the cache and other callers' copies intact,
// on a cache hit and on a fetch.
func TestManager_Value_MutationIsolation(t *testing.T) {
	t.Parallel()

	t.Run("map payload", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		provider := &blockingProvider[map[string]string]{payload: func() map[string]string { return map[string]string{"user": "u"} }}
		mgr, err := secrets.New[map[string]string](provider)
		require.NoError(t, err)

		fetched, err := mgr.Value(ctx, "key-a", true)
		require.NoError(t, err)
		fetched.Value["user"] = "changed"

		hit, err := mgr.Value(ctx, "key-a", false)
		require.NoError(t, err)
		require.Equal(t, "u", hit.Value["user"])
		hit.Value["user"] = "changed-again"

		cached, err := mgr.ValueShared(ctx, "key-a", false)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"user": "u"}, cached.Value)
	})

	t.Run("any payload holding bytes", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		provider := &blockingProvider[any]{payload: func() any { return []byte("raw") }}
		mgr, err := secrets.New[any](provider)
		require.NoError(t, err)

		fetched, err := mgr.Value(ctx, "key-a", true)
		require.NoError(t, err)
		fetched.Value.([]byte)[0] = 'X'

		hit, err := mgr.Value(ctx, "key-a", false)
		require.NoError(t, err)
		require.Equal(t, "raw", string(hit.Value.([]byte)))
		hit.Value.([]byte)[0] = 'Y'

		cached, err := mgr.ValueShared(ctx, "key-a", false)
		require.NoError(t, err)
		require.Equal(t, "raw", string(cached.Value.([]byte)))
	})
}

// TestManager_Value_ConcurrentWithClears reads and clears Value copies while
// Delete, Save, ClearCache and update cycles clear cached entries. Run with
// -race.
func TestManager_Value_ConcurrentWithClears(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	keys := []string{"key-0", "key-1", "key-2"}
	inner, err := secretmemory.New(map[string]string{"key-0": "v", "key-1": "v", "key-2": "v"})
	require.NoError(t, err)
	mgr, err := secrets.New[string](inner, secrets.WithMaxRetries(1),
		secrets.WithExponentialConfig(retry.ExponentialConfig{BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Factor: 1}))
	require.NoError(t, err)

	const iterations = 200
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			for i := range iterations {
				v, err := mgr.Value(ctx, keys[i%len(keys)], true)
				if err == nil {
					_, _, _ = v.Key, v.Version, v.Value
					v.Clear()
				}
			}
		})
	}
	wg.Go(func() {
		for i := range iterations {
			key := keys[i%len(keys)]
			_ = mgr.Delete(ctx, key)
			_ = mgr.Save(ctx, key, "v")
			mgr.ClearCache(ctx)
		}
	})
	wg.Go(func() {
		for range iterations {
			_ = mgr.RunUpdateCycle(ctx)
		}
	})
	wg.Wait()
}

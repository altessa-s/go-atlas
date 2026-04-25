// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq_test

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/uniq"

	goredis "github.com/redis/go-redis/v9"
)

// Compile-time check that Uniquer carries the new CAS surface.
var _ = (uniq.Uniquer)(nil)

func TestTryAdd_EmptyKey(t *testing.T) {
	t.Parallel()

	u := uniq.NewWithNoop()
	ok, err := u.TryAdd(t.Context(), "")
	require.False(t, ok)
	require.ErrorIs(t, err, uniq.ErrInvalidKey)
}

func TestTryAdd_KeyTooLong(t *testing.T) {
	t.Parallel()

	u := uniq.NewWithNoop()
	ok, err := u.TryAdd(t.Context(), strings.Repeat("a", 1025))
	require.False(t, ok)
	require.ErrorIs(t, err, uniq.ErrInvalidKey)
}

func TestTryAddWithValue_EmptyKey(t *testing.T) {
	t.Parallel()

	u := uniq.NewWithNoop()
	ok, err := u.TryAddWithValue(t.Context(), "", "v")
	require.False(t, ok)
	require.ErrorIs(t, err, uniq.ErrInvalidKey)
}

func TestTryAddWithValue_SerializationError(t *testing.T) {
	t.Parallel()

	// JSON serializer can't encode a channel — verifies that
	// serialization failure short-circuits before any storage call.
	u := uniq.NewWithNoop()
	ok, err := u.TryAddWithValue(t.Context(), "k", make(chan int))
	require.False(t, ok)
	require.ErrorIs(t, err, uniq.ErrSerializationFailed)
}

// TestTryAdd_RegressionRace is the load-bearing test for the race fix:
// many goroutines try to acquire the same key concurrently. With CAS
// semantics on the storage layer, exactly one must win.
func TestTryAdd_RegressionRace(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	u := uniq.NewWithRedis(client)

	const concurrency = 50
	var (
		acquired atomic.Int32
		errCh    = make(chan error, concurrency)
		wg       sync.WaitGroup
	)
	for range concurrency {
		wg.Go(func() {
			ok, err := u.TryAdd(t.Context(), "race-key")
			if err != nil {
				errCh <- err
				return
			}
			if ok {
				acquired.Add(1)
			}
		})
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	// Exactly one goroutine wins. Anything else means the race is open.
	require.Equal(t, int32(1), acquired.Load(),
		"expected exactly one TryAdd() to acquire under concurrent load; race is not closed")
}

// TestTryAddWithValue_DoesNotOverwrite documents the (false, nil) path
// at the *Uniq level: the caller's value is dropped silently when the
// key is already present, and the original value remains readable via
// GetValue.
func TestTryAddWithValue_DoesNotOverwrite(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	u := uniq.NewWithRedis(client)
	ctx := t.Context()

	require.NoError(t, u.AddWithValue(ctx, "k", "original"))

	ok, err := u.TryAddWithValue(ctx, "k", "loser")
	require.NoError(t, err)
	require.False(t, ok)

	var got string
	require.NoError(t, u.GetValue(ctx, "k", &got))
	require.Equal(t, "original", got)
}

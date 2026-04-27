// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/uniq"
	"github.com/altessa-s/go-atlas/data/uniq/providers"

	goredis "github.com/redis/go-redis/v9"
)

// Compile-time check that Uniquer carries the new CAS surface.
var _ = (uniq.Uniquer)(nil)

func TestTryAdd_EmptyKey(t *testing.T) {
	t.Parallel()

	u := uniq.NewWithNoop()
	ok, err := u.TryAdd(t.Context(), "", 0)
	require.False(t, ok)
	require.ErrorIs(t, err, uniq.ErrInvalidKey)
}

func TestTryAdd_KeyTooLong(t *testing.T) {
	t.Parallel()

	u := uniq.NewWithNoop()
	ok, err := u.TryAdd(t.Context(), strings.Repeat("a", 1025), 0)
	require.False(t, ok)
	require.ErrorIs(t, err, uniq.ErrInvalidKey)
}

func TestTryAddWithValue_EmptyKey(t *testing.T) {
	t.Parallel()

	u := uniq.NewWithNoop()
	ok, err := u.TryAddWithValue(t.Context(), "", "v", 0)
	require.False(t, ok)
	require.ErrorIs(t, err, uniq.ErrInvalidKey)
}

func TestTryAddWithValue_SerializationError(t *testing.T) {
	t.Parallel()

	// JSON serializer can't encode a channel — verifies that
	// serialization failure short-circuits before any storage call.
	u := uniq.NewWithNoop()
	ok, err := u.TryAddWithValue(t.Context(), "k", make(chan int), 0)
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
			ok, err := u.TryAdd(t.Context(), "race-key", 0)
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

	ok, err := u.TryAddWithValue(ctx, "k", "loser", 0)
	require.NoError(t, err)
	require.False(t, ok)

	var got string
	require.NoError(t, u.GetValue(ctx, "k", &got))
	require.Equal(t, "original", got)
}

// fakeTtlProvider records the ttl value seen by the most recent
// TryAdd / TryAddWithValue call. Used to verify that *Uniq passes the
// per-call ttl through to the provider unchanged.
type fakeTtlProvider struct {
	lastTtl time.Duration
}

func (p *fakeTtlProvider) Add(_ context.Context, _ string) error                    { return nil }
func (p *fakeTtlProvider) AddWithValue(_ context.Context, _ string, _ []byte) error { return nil }
func (p *fakeTtlProvider) Exist(_ context.Context, _ string) (bool, error)          { return false, nil }
func (p *fakeTtlProvider) GetValue(_ context.Context, _ string) ([]byte, error)     { return nil, nil }
func (p *fakeTtlProvider) Remove(_ context.Context, _ string) error                 { return nil }
func (p *fakeTtlProvider) Clear(_ context.Context) error                            { return nil }

func (p *fakeTtlProvider) TryAdd(_ context.Context, _ string, ttl time.Duration) (bool, error) {
	p.lastTtl = ttl
	return true, nil
}

func (p *fakeTtlProvider) TryAddWithValue(_ context.Context, _ string, _ []byte, ttl time.Duration) (bool, error) {
	p.lastTtl = ttl
	return true, nil
}

var _ providers.Provider = (*fakeTtlProvider)(nil)

func TestTryAdd_PerCallTtl_PassedToProvider(t *testing.T) {
	t.Parallel()

	prov := &fakeTtlProvider{}
	u := uniq.New(prov)

	_, err := u.TryAdd(t.Context(), "k", 7*time.Second)
	require.NoError(t, err)
	require.Equal(t, 7*time.Second, prov.lastTtl,
		"*Uniq.TryAdd must pass ttl through unchanged")

	_, err = u.TryAddWithValue(t.Context(), "k", "v", 9*time.Second)
	require.NoError(t, err)
	require.Equal(t, 9*time.Second, prov.lastTtl,
		"*Uniq.TryAddWithValue must pass ttl through unchanged")

	_, err = u.TryAdd(t.Context(), "k", 0)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), prov.lastTtl,
		"zero ttl must propagate as zero (provider decides fallback)")
}

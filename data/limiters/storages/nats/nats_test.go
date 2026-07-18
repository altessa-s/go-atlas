// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	limitnats "github.com/altessa-s/go-atlas/data/limiters/storages/nats"
)

func setupProvider(tb testing.TB) *limitnats.Provider {
	tb.Helper()
	ns := testhelpers.StartNATSServer(tb)
	_, js := testhelpers.ConnectJetStream(tb, ns)

	provider, err := limitnats.New(js, limitnats.WithBucket(tb.Name()))
	require.NoError(tb, err)
	return provider
}

func TestNew(t *testing.T) {
	provider := setupProvider(t)
	require.NotNil(t, provider, "New() returned nil")
}

func TestNew_NilJetStream(t *testing.T) {
	_, err := limitnats.New(nil)
	require.Error(t, err, "New(nil) should return error")
}

// TestNew_ReplicasWiredToBucketConfig pins that WithReplicas reaches the
// jetstream.KeyValueConfig used for bucket creation. A capture double is used
// because a single-node test server cannot create buckets with replicas > 1.
func TestNew_ReplicasWiredToBucketConfig(t *testing.T) {
	t.Parallel()

	capture := &testhelpers.JetStreamKVCapture{}

	_, err := limitnats.New(capture,
		limitnats.WithBucket("rate-limiter-replicas"),
		limitnats.WithReplicas(3))
	require.NoError(t, err)

	require.Equal(t, "rate-limiter-replicas", capture.KVConfig.Bucket)
	require.Equal(t, 3, capture.KVConfig.Replicas)
}

func TestProvider_Allow_UnderLimit(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	info, err := provider.Allow(ctx, "key1", 5, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(4), info.Remaining)
}

func TestProvider_Allow_ExceedsLimit(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	for range 3 {
		_, err := provider.Allow(ctx, "key1", 3, time.Minute)
		require.NoError(t, err)
	}

	info, err := provider.Allow(ctx, "key1", 3, time.Minute)
	require.ErrorIs(t, err, storages.ErrLimitExceeded)
	require.Equal(t, int64(0), info.Remaining)
}

func TestProvider_Allow_DifferentKeys(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	_, err := provider.Allow(ctx, "key1", 1, time.Minute)
	require.NoError(t, err)

	_, err = provider.Allow(ctx, "key2", 1, time.Minute)
	require.NoError(t, err)
}

func TestProvider_Reset(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	for range 3 {
		_, _ = provider.Allow(ctx, "key1", 3, time.Minute)
	}

	err := provider.Reset(ctx, "key1")
	require.NoError(t, err)

	_, err = provider.Allow(ctx, "key1", 3, time.Minute)
	require.NoError(t, err)
}

func TestProvider_Reset_NonExistent(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	err := provider.Reset(ctx, "nonexistent")
	require.NoError(t, err)
}

// TestProvider_Allow_ConcurrentBurst is a regression test for an earlier bug
// where Allow used Get + Put without revision checks, so concurrent writers
// silently overwrote each other and many requests passed even though the
// counter should have been at the limit. The fix is revision-based CAS via
// jetstream Create/Update; this test verifies the property by firing many
// goroutines at the same key and asserting the allowed count never exceeds
// the configured limit.
func TestProvider_Allow_ConcurrentBurst(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	const limit, goroutines = 10, 30

	var (
		wg       sync.WaitGroup
		allowed  atomic.Int64
		exceeded atomic.Int64
		other    atomic.Int64
		start    = make(chan struct{})
	)
	for range goroutines {
		wg.Go(func() {
			<-start
			_, err := provider.Allow(ctx, "burst-key", limit, time.Minute)
			switch {
			case err == nil:
				allowed.Add(1)
			case errors.Is(err, storages.ErrLimitExceeded):
				exceeded.Add(1)
			default:
				// CAS retries can be exhausted under heavy contention; that is
				// preferable to silently accepting the request, so it counts
				// as "not allowed" for limit-enforcement purposes.
				other.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()

	require.LessOrEqual(t, allowed.Load(), int64(limit),
		"concurrent CAS must never let more than `limit` requests through (allowed=%d, exceeded=%d, other=%d)",
		allowed.Load(), exceeded.Load(), other.Load())
	require.Equal(t, int64(goroutines), allowed.Load()+exceeded.Load()+other.Load(),
		"every goroutine must have been accounted for")
}

func TestProvider_Allow_Panics(t *testing.T) {
	provider := setupProvider(t)
	ctx := t.Context()

	tests := []struct {
		name string
		fn   func()
	}{
		{"nil context", func() { provider.Allow(nil, "k", 1, time.Second) }}, //nolint:staticcheck,SA1012
		{"empty key", func() { provider.Allow(ctx, "", 1, time.Second) }},
		{"zero limit", func() { provider.Allow(ctx, "k", 0, time.Second) }},
		{"zero period", func() { provider.Allow(ctx, "k", 1, 0) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected panic")
				}
			}()
			tt.fn()
		})
	}
}

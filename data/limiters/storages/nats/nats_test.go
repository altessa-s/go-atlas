// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
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
	// A call conflicts only when another call committed in between, so the
	// default attempt budget outlasts a burst of this size.
	require.Zero(t, other.Load(), "contention must be absorbed by retries")
	require.Equal(t, int64(limit), allowed.Load(), "the window must fill up exactly")
}

// TestProvider_Allow_SustainedContention keeps 32 goroutines racing one key
// for several calls each: every call ends in a decision, and the window
// holds exactly limit requests.
func TestProvider_Allow_SustainedContention(t *testing.T) {
	t.Parallel()
	provider := setupProvider(t)
	ctx := t.Context()

	const limit, goroutines, calls = 40, 32, 8

	var (
		allowed  atomic.Int64
		exceeded atomic.Int64
		mu       sync.Mutex
		failures []error
	)
	workers := make([]int, goroutines)
	require.NoError(t, concurrency.Process(ctx, workers, func(ctx context.Context, _ int) error {
		for range calls {
			_, err := provider.Allow(ctx, "hot-key", limit, time.Minute)
			switch {
			case err == nil:
				allowed.Add(1)
			case errors.Is(err, storages.ErrLimitExceeded):
				exceeded.Add(1)
			default:
				mu.Lock()
				failures = append(failures, err)
				mu.Unlock()
			}
		}
		return nil
	}, concurrency.WithConcurrency[int](goroutines)))

	require.Empty(t, failures)
	require.Equal(t, int64(limit), allowed.Load())
	require.Equal(t, int64(goroutines*calls-limit), exceeded.Load())
}

// conflictingJetStream hands out KeyValue buckets in which another writer
// updates every key right after it is read, so each CAS write of the
// provider loses the race deterministically.
type conflictingJetStream struct {
	jetstream.JetStream
}

func (c conflictingJetStream) KeyValue(ctx context.Context, bucket string) (jetstream.KeyValue, error) {
	kv, err := c.JetStream.KeyValue(ctx, bucket)
	if err != nil {
		return nil, err
	}
	return &conflictingKV{KeyValue: kv}, nil
}

func (c conflictingJetStream) CreateKeyValue(ctx context.Context, cfg jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
	kv, err := c.JetStream.CreateKeyValue(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &conflictingKV{KeyValue: kv}, nil
}

type conflictingKV struct {
	jetstream.KeyValue
	reads atomic.Int64
}

func (kv *conflictingKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	entry, err := kv.KeyValue.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	kv.reads.Add(1)
	if _, err := kv.KeyValue.Put(ctx, key, entry.Value()); err != nil {
		return nil, err
	}
	return entry, nil
}

func TestProvider_Allow_ContentionExhaustsAttempts(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	// Seed the key, so every later attempt reads an entry and then conflicts.
	seed, err := limitnats.New(js, limitnats.WithBucket("contended"))
	require.NoError(t, err)
	_, err = seed.Allow(t.Context(), "key", 10, time.Minute)
	require.NoError(t, err)

	const attempts = 3
	cjs := conflictingJetStream{JetStream: js}
	provider, err := limitnats.New(cjs, limitnats.WithBucket("contended"),
		limitnats.WithMaxCASAttempts(attempts), limitnats.WithMaxCASBackoff(time.Millisecond))
	require.NoError(t, err)
	kv, ok := provider.KV().(*conflictingKV)
	require.True(t, ok)

	info, err := provider.Allow(t.Context(), "key", 10, time.Minute)
	require.ErrorIs(t, err, limitnats.ErrContention)
	require.ErrorIs(t, err, jetstream.ErrKeyExists, "the last conflict stays visible")
	require.NotErrorIs(t, err, storages.ErrLimitExceeded, "contention is not a limit decision")
	require.Nil(t, info)
	require.Equal(t, int64(attempts), kv.reads.Load(), "one read per attempt")

	info, err = seed.Allow(t.Context(), "key", 10, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(8), info.Remaining, "the contended call must not have been counted")
}

func TestProvider_Allow_ContentionStopsAtContextEnd(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	seed, err := limitnats.New(js, limitnats.WithBucket("canceled"))
	require.NoError(t, err)
	_, err = seed.Allow(t.Context(), "key", 10, time.Minute)
	require.NoError(t, err)

	provider, err := limitnats.New(conflictingJetStream{JetStream: js}, limitnats.WithBucket("canceled"),
		limitnats.WithMaxCASAttempts(1_000_000), limitnats.WithMaxCASBackoff(5*time.Millisecond))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err = provider.Allow(ctx, "key", 10, time.Minute)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, limitnats.ErrContention)
}

// TestProvider_Allow_ResetTime asserts Reset is the second the oldest request
// in the window leaves it — on the first admission into an empty window the
// admitted request itself.
func TestProvider_Allow_ResetTime(t *testing.T) {
	t.Parallel()
	provider := setupProvider(t)
	ctx := t.Context()
	const period = 30 * time.Second

	before := time.Now()
	first, err := provider.Allow(ctx, "key", 2, period)
	after := time.Now()
	require.NoError(t, err)
	lo, hi := before.Add(period).UnixMilli()/1000, after.Add(period).UnixMilli()/1000
	require.GreaterOrEqual(t, first.Reset, lo)
	require.LessOrEqual(t, first.Reset, hi)

	second, err := provider.Allow(ctx, "key", 2, period)
	require.NoError(t, err)
	require.Equal(t, first.Reset, second.Reset, "the first request is still the oldest")

	rejected, err := provider.Allow(ctx, "key", 2, period)
	require.ErrorIs(t, err, storages.ErrLimitExceeded)
	require.Equal(t, first.Reset, rejected.Reset)

	require.NoError(t, provider.Reset(ctx, "key"))
	before = time.Now()
	fresh, err := provider.Allow(ctx, "key", 2, period)
	after = time.Now()
	require.NoError(t, err)
	require.GreaterOrEqual(t, fresh.Reset, before.Add(period).UnixMilli()/1000)
	require.LessOrEqual(t, fresh.Reset, after.Add(period).UnixMilli()/1000)
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

// TestProvider_Allow_SubMillisecondPeriod pins that a period shorter than a
// millisecond keeps a 1ms window instead of a 0ms one that never limits.
func TestProvider_Allow_SubMillisecondPeriod(t *testing.T) {
	t.Parallel()
	provider := setupProvider(t)
	ctx := t.Context()

	_, err := provider.Allow(ctx, "sub-ms", 1, 500*time.Microsecond)
	require.NoError(t, err)

	entry, err := provider.KV().Get(ctx, "sub-ms")
	require.NoError(t, err)
	var stored struct {
		Window int64 `json:"window"`
	}
	require.NoError(t, json.Unmarshal(entry.Value(), &stored))
	require.Equal(t, int64(1), stored.Window)
}

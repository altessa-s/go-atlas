// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
)

func TestProvider_Allow_UnderLimit(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	info, err := p.Allow(ctx, "key1", 5, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(4), info.Remaining)
}

func TestProvider_Allow_ExceedsLimit(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	for range 3 {
		_, err := p.Allow(ctx, "key1", 3, time.Minute)
		require.NoError(t, err)
	}

	info, err := p.Allow(ctx, "key1", 3, time.Minute)
	require.ErrorIs(t, err, storages.ErrLimitExceeded)
	require.Equal(t, int64(0), info.Remaining)
}

func TestProvider_Allow_DifferentKeys(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	_, err := p.Allow(ctx, "key1", 1, time.Minute)
	require.NoError(t, err)

	_, err = p.Allow(ctx, "key2", 1, time.Minute)
	require.NoError(t, err)
}

func TestProvider_Reset(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	// Exhaust the limit
	for range 3 {
		_, _ = p.Allow(ctx, "key1", 3, time.Minute)
	}

	err := p.Reset(ctx, "key1")
	require.NoError(t, err)

	// Should be allowed again
	_, err = p.Allow(ctx, "key1", 3, time.Minute)
	require.NoError(t, err)
}

func TestProvider_Reset_NonExistent(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	err := p.Reset(ctx, "nonexistent")
	require.NoError(t, err)
}

func TestProvider_Close(t *testing.T) {
	p := memory.New()
	require.NoError(t, p.Close())
}

func TestProvider_RunCleanup(t *testing.T) {
	p := memory.New(memory.WithMaxIdleTime(1 * time.Millisecond))
	ctx := t.Context()

	_, _ = p.Allow(ctx, "key1", 10, time.Minute)

	time.Sleep(5 * time.Millisecond)
	p.RunCleanup()

	// After cleanup, key should be gone; new request should succeed
	info, err := p.Allow(ctx, "key1", 10, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(9), info.Remaining)
}

func TestProvider_Allow_Panics(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	tests := []struct {
		name string
		fn   func()
	}{
		{"nil context", func() { p.Allow(nil, "k", 1, time.Second) }}, //nolint:staticcheck,SA1012
		{"empty key", func() { p.Allow(ctx, "", 1, time.Second) }},
		{"zero limit", func() { p.Allow(ctx, "k", 0, time.Second) }},
		{"zero period", func() { p.Allow(ctx, "k", 1, 0) }},
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

// TestProvider_MaxBuckets_EvictsLRU is the regression guard for the
// unbounded-map fix. With MaxBuckets=2, adding a third distinct key must
// evict the oldest bucket (key1) rather than letting the map grow
// indefinitely. The cap is the only memory bound when no cleanup
// scheduler is attached.
func TestProvider_MaxBuckets_EvictsLRU(t *testing.T) {
	p := memory.New(memory.WithMaxBuckets(2))
	ctx := t.Context()

	_, err := p.Allow(ctx, "key1", 10, time.Minute)
	require.NoError(t, err)
	// Tiny sleep so the next Allow gets a strictly-later lastUsed —
	// without this, two consecutive sub-microsecond calls can tie and
	// the eviction picks either bucket, making the assertion flaky.
	time.Sleep(2 * time.Millisecond)

	_, err = p.Allow(ctx, "key2", 10, time.Minute)
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)

	_, err = p.Allow(ctx, "key3", 10, time.Minute)
	require.NoError(t, err)

	// key1 was least-recently-used and must have been evicted. Re-adding
	// it should give a fresh limit budget (Remaining = limit-1), not the
	// stale "already used 1" value.
	info, err := p.Allow(ctx, "key1", 10, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(9), info.Remaining,
		"after eviction, re-adding key1 must start from a fresh bucket — proves key1's old state was dropped, not retained")
}

// TestProvider_MaxBuckets_DisabledKeepsLegacyBehavior pins the
// backwards-compat path: setting MaxBuckets <= 0 disables the cap so
// memory grows unbounded (matches pre-fix behavior for callers who
// explicitly opt out).
func TestProvider_MaxBuckets_DisabledKeepsLegacyBehavior(t *testing.T) {
	p := memory.New(memory.WithMaxBuckets(0))
	ctx := t.Context()

	// Add more keys than DefaultMaxBuckets would allow if the cap were on.
	for i := range 10 {
		_, err := p.Allow(ctx, "k"+string(rune('a'+i)), 10, time.Minute)
		require.NoError(t, err)
	}
	// No assertion on internal map size (it's unexported) — the test
	// just confirms the path compiles and runs without panic.
}

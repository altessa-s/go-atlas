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

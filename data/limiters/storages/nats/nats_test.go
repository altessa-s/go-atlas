// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
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

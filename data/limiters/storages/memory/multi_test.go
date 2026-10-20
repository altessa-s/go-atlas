// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
	"github.com/altessa-s/go-atlas/data/limiters/storages/storagetest"
)

func TestProvider_MultiStorageContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []memory.Option
	}{
		{name: "default"},
		{name: "shards=8", opts: []memory.Option{memory.WithShards(8)}},
		{name: "reject", opts: []memory.Option{memory.WithShards(8), memory.WithEviction(memory.EvictionReject)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storagetest.Run(t, func(testing.TB) storages.MultiStorage { return memory.New(tc.opts...) })
		})
	}
}

// TestProvider_SequentialAllowBurnsSlot documents the problem AllowAll
// solves: two Allow calls take the host slot before the global key refuses.
func TestProvider_SequentialAllowBurnsSlot(t *testing.T) {
	t.Parallel()
	p := memory.New()
	ctx := t.Context()
	_, err := p.Allow(ctx, "global", 1, time.Minute)
	require.NoError(t, err)

	_, err = p.Allow(ctx, "host", 1, time.Minute)
	require.NoError(t, err)
	_, err = p.Allow(ctx, "global", 1, time.Minute)
	require.ErrorIs(t, err, storages.ErrLimitExceeded)
	_, err = p.Allow(ctx, "host", 1, time.Minute)
	require.ErrorIs(t, err, storages.ErrLimitExceeded, "the refused request burned the host slot")

	_, err = storages.AllowAll(ctx, p,
		storages.Limit{Key: "host2", Limit: 1, Period: time.Minute},
		storages.Limit{Key: "global", Limit: 1, Period: time.Minute},
	)
	require.ErrorIs(t, err, storages.ErrLimitExceeded)
	_, err = p.Allow(ctx, "host2", 1, time.Minute)
	require.NoError(t, err, "AllowAll must not burn the host slot")
}

func TestProvider_AllowAll_LargeCall(t *testing.T) {
	t.Parallel()
	p := memory.New(memory.WithShards(4))
	ctx := t.Context()
	limits := make([]storages.Limit, 12)
	for i := range limits {
		limits[i] = storages.Limit{Key: "k" + strconv.Itoa(i), Limit: 2, Period: time.Minute}
	}

	for range 2 {
		infos, err := p.AllowAll(ctx, limits)
		require.NoError(t, err)
		require.Len(t, infos, len(limits))
	}
	_, err := p.AllowAll(ctx, limits)
	keyErr, ok := errors.AsType[*storages.KeyError](err)
	require.True(t, ok, "%v", err)
	require.Equal(t, 0, keyErr.Index)

	dup := append(limits[:len(limits):len(limits)], limits[3])
	_, err = p.AllowAll(ctx, dup)
	require.ErrorIs(t, err, storages.ErrDuplicateKey)
}

func TestProvider_AllowAll_Panics(t *testing.T) {
	t.Parallel()
	p := memory.New()
	ctx := t.Context()
	ok := storages.Limit{Key: "k", Limit: 1, Period: time.Second}

	tests := []struct {
		name string
		fn   func()
	}{
		{"nil context", func() { _, _ = p.AllowAll(nil, []storages.Limit{ok}) }}, //nolint:staticcheck // SA1012: nil context under test
		{"no limits", func() { _, _ = p.AllowAll(ctx, nil) }},
		{"empty key", func() { _, _ = p.AllowAll(ctx, []storages.Limit{ok, {Limit: 1, Period: time.Second}}) }},
		{"zero limit", func() { _, _ = p.AllowAll(ctx, []storages.Limit{{Key: "k", Period: time.Second}}) }},
		{"zero period", func() { _, _ = p.AllowAll(ctx, []storages.Limit{{Key: "k", Limit: 1}}) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Panics(t, tc.fn)
		})
	}
}

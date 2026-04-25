// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/noop"
)

func TestNew(t *testing.T) {
	p := noop.New()
	require.NotNil(t, p, "New() returned nil")
}

func TestProvider_Lock(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	lk, err := p.Lock(ctx, "test-key")
	require.NoError(t, err)
	require.NotNil(t, lk, "Lock() returned nil")

	info, err := lk.GetLockInfo(ctx)
	require.NoError(t, err)
	require.Equal(t, "test-key", info.Key)
	require.Equal(t, "nop", info.Owner)
}

func TestProvider_Lock_AfterClose(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	_ = p.Close(ctx)

	_, err := p.Lock(ctx, "test-key")
	require.Error(t, err, "Lock() after Close() should return error")
}

func TestProvider_GetLockInfo(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	info, err := p.GetLockInfo(ctx, "any-key")
	require.NoError(t, err)
	require.Equal(t, "any-key", info.Key)
	require.Equal(t, "nop", info.Owner)
	require.False(t, info.IsStale, "IsStale should be false")
	require.Equal(t, uint64(0), info.FencingToken)
}

func TestLock_Release(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	lk, _ := p.Lock(ctx, "key")
	require.NoError(t, lk.Release(ctx))
}

func TestLock_Release_CanceledContext(t *testing.T) {
	p := noop.New()

	lk, _ := p.Lock(t.Context(), "key")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.Error(t, lk.Release(ctx), "Release() with canceled context should return error")
}

func TestProvider_Close(t *testing.T) {
	p := noop.New()
	require.NoError(t, p.Close(t.Context()))
}

func TestProvider_Lock_DifferentKeys(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	keys := []string{"key-a", "key-b", "key-c"}
	for _, key := range keys {
		lk, err := p.Lock(ctx, key)
		require.NoError(t, err)
		info, _ := lk.GetLockInfo(ctx)
		require.Equal(t, key, info.Key)
		_ = lk.Release(ctx)
	}
}

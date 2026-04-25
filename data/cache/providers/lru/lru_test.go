// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/providers"
)

func newTestProvider(t *testing.T) *Provider {
	t.Helper()
	p, err := New(100)
	require.NoError(t, err)
	return p
}

func TestProvider_SaveGet(t *testing.T) {
	p := newTestProvider(t)
	ctx := t.Context()

	err := p.Save(ctx, "key", []byte("value"), time.Hour)
	require.NoError(t, err)

	v, err := p.Get(ctx, "key")
	require.NoError(t, err)
	require.Equal(t, "value", string(v))
}

func TestProvider_TTLExpiry(t *testing.T) {
	p := newTestProvider(t)
	ctx := t.Context()

	err := p.Save(ctx, "key", []byte("value"), time.Nanosecond)
	require.NoError(t, err)

	// Wait for expiry
	time.Sleep(time.Millisecond)

	_, err = p.Get(ctx, "key")
	require.ErrorIs(t, err, providers.ErrMissing)
}

func TestProvider_Exists(t *testing.T) {
	p := newTestProvider(t)
	ctx := t.Context()

	_ = p.Save(ctx, "key", []byte("val"), time.Hour)

	exists, err := p.Exists(ctx, "key")
	require.NoError(t, err)
	require.True(t, exists)

	exists, _ = p.Exists(ctx, "missing")
	require.False(t, exists, "expected false for missing key")
}

func TestProvider_Delete(t *testing.T) {
	p := newTestProvider(t)
	ctx := t.Context()

	_ = p.Save(ctx, "key", []byte("val"), time.Hour)
	_ = p.Delete(ctx, "key")

	_, err := p.Get(ctx, "key")
	require.ErrorIs(t, err, providers.ErrMissing)
}

func TestProvider_DeleteMany(t *testing.T) {
	p := newTestProvider(t)
	ctx := t.Context()

	_ = p.Save(ctx, "k1", []byte("a"), time.Hour)
	_ = p.Save(ctx, "k2", []byte("b"), time.Hour)

	_ = p.DeleteMany(ctx, "k1", "k2")

	_, err := p.Get(ctx, "k1")
	require.ErrorIs(t, err, providers.ErrMissing)
	_, err = p.Get(ctx, "k2")
	require.ErrorIs(t, err, providers.ErrMissing)
}

func TestProvider_Get_Missing(t *testing.T) {
	p := newTestProvider(t)
	_, err := p.Get(t.Context(), "missing")
	require.ErrorIs(t, err, providers.ErrMissing)
}

// TestProvider_ConcurrentSaveGetExpiry verifies that a concurrent Save does not
// lose data when another goroutine reads an expired item for the same key.
// Before the fix, Get would call Remove on expired items, which could delete a
// fresh value stored by a concurrent Save between the expiry check and Remove.
func TestProvider_ConcurrentSaveGetExpiry(t *testing.T) {
	p := newTestProvider(t)
	ctx := t.Context()

	// Store an item that expires immediately.
	_ = p.Save(ctx, "k", []byte("old"), time.Nanosecond)
	time.Sleep(time.Millisecond) // ensure expired

	// A concurrent Save stores a fresh value for the same key.
	_ = p.Save(ctx, "k", []byte("fresh"), time.Hour)

	// A Get that observes the expired item must NOT remove the fresh value.
	v, err := p.Get(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, "fresh", string(v))
}

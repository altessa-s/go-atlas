// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/providers"
	"github.com/altessa-s/go-atlas/data/cache/providers/noop"
)

// mockProvider is a test-local cache provider with in-memory store.
type mockProvider struct {
	store     map[string][]byte
	lastTTL   time.Duration
	saveErr   error
	getErr    error
	deleteErr error
}

func newMockProvider() *mockProvider {
	return &mockProvider{store: make(map[string][]byte)}
}

func (m *mockProvider) Save(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.lastTTL = ttl
	m.store[key] = value
	return nil
}

func (m *mockProvider) Get(_ context.Context, key string) ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	v, ok := m.store[key]
	if !ok {
		return nil, providers.ErrMissing
	}
	return v, nil
}

func (m *mockProvider) Delete(_ context.Context, key string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.store, key)
	return nil
}

func (m *mockProvider) DeleteMany(_ context.Context, keys ...string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	for _, k := range keys {
		delete(m.store, k)
	}
	return nil
}

func (m *mockProvider) Exists(_ context.Context, key string) (bool, error) {
	_, ok := m.store[key]
	return ok, nil
}

func TestNew(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	require.NotNil(t, c, "New returned nil")
	require.Equal(t, p, c.provider, "provider not set")
}

func TestNew_NilProviderPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil provider")
		}
	}()
	New(nil)
}

func TestNewNoop(t *testing.T) {
	c := NewNoop()
	require.NotNil(t, c, "NewNoop returned nil")
	_, ok := c.provider.(*noop.Provider)
	require.True(t, ok, "expected noop provider")
}

func TestSave(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	err := c.Save(ctx, "key1", "hello")
	require.NoError(t, err)
	_, ok := p.store["key1"]
	require.True(t, ok, "key not stored in provider")
}

func TestGet_Hit(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	_ = c.Save(ctx, "key1", "hello")

	var result string
	err := c.Get(ctx, "key1", &result)
	require.NoError(t, err)
	require.Equal(t, "hello", result)
}

func TestGet_Miss(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	var result string
	err := c.Get(ctx, "missing", &result)
	require.ErrorIs(t, err, ErrMissing)
}

func TestExists(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	_ = c.Save(ctx, "key1", "hello")

	exists, err := c.Exists(ctx, "key1")
	require.NoError(t, err)
	require.True(t, exists, "expected true")

	exists, err = c.Exists(ctx, "missing")
	require.NoError(t, err)
	require.False(t, exists, "expected false")
}

func TestDelete(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	_ = c.Save(ctx, "key1", "hello")
	err := c.Delete(ctx, "key1")
	require.NoError(t, err)

	var result string
	err = c.Get(ctx, "key1", &result)
	require.ErrorIs(t, err, ErrMissing)
}

func TestDeleteMany(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	_ = c.Save(ctx, "k1", "a")
	_ = c.Save(ctx, "k2", "b")

	err := c.DeleteMany(ctx, "k1", "k2")
	require.NoError(t, err)
	require.Len(t, p.store, 0)
}

func TestGetWithFallback_CacheHit(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	_ = c.Save(ctx, "key1", "cached")

	var result string
	err := c.GetWithFallback(ctx, "key1", &result, func() (any, time.Duration, error) {
		t.Error("fallback should not be called on cache hit")
		return "fallback", TTLUseDefault, nil
	})
	require.NoError(t, err)
	require.Equal(t, "cached", result)
}

func TestGetWithFallback_CacheMiss_FallbackHit(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	var result string
	err := c.GetWithFallback(ctx, "key1", &result, func() (any, time.Duration, error) {
		return "from-fallback", TTLUseDefault, nil
	})
	require.NoError(t, err)
	require.Equal(t, "from-fallback", result)
}

func TestGetWithFallback_FallbackError(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	wantErr := errors.New("db error")
	var result string
	err := c.GetWithFallback(ctx, "key1", &result, func() (any, time.Duration, error) {
		return nil, TTLUseDefault, wantErr
	})
	require.ErrorIs(t, err, wantErr)
}

func TestGetWithFallback_Singleflight(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	calls := 0
	fallback := func() (any, time.Duration, error) {
		calls++
		return "value", TTLUseDefault, nil
	}

	// Sequential calls — singleflight deduplicates concurrent ones,
	// but the second should hit cache.
	var r1, r2 string
	_ = c.GetWithFallback(ctx, "sf-key", &r1, fallback)
	_ = c.GetWithFallback(ctx, "sf-key", &r2, fallback)

	require.Equal(t, "value", r1)
	require.Equal(t, "value", r2)
	// The first call triggers fallback, second gets cache hit.
	require.Equal(t, 1, calls, "fallback called unexpected number of times")
}

func TestEmptyKeyPanics(t *testing.T) {
	c := NewNoop()
	ctx := t.Context()

	panics := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("%s: expected panic for empty key", name)
			}
		}()
		fn()
	}

	panics("Save", func() { _ = c.Save(ctx, "", "v") })
	panics("Get", func() { _ = c.Get(ctx, "", new(string)) })
	panics("Delete", func() { _ = c.Delete(ctx, "") })
	panics("Exists", func() { _, _ = c.Exists(ctx, "") })
	panics("GetWithFallback", func() {
		_ = c.GetWithFallback(ctx, "", new(string), func() (any, time.Duration, error) {
			return nil, 0, nil
		})
	})
}

func TestDeleteMany_NoKeysPanics(t *testing.T) {
	c := NewNoop()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for no keys")
		}
	}()
	_ = c.DeleteMany(t.Context())
}

func TestGetWithFallback_NegativeCaching_Disabled(t *testing.T) {
	p := newMockProvider()
	c := New(p) // no WithNegativeTtl
	ctx := t.Context()

	calls := 0
	fallback := func() (any, time.Duration, error) {
		calls++
		return (*string)(nil), TTLUseDefault, nil
	}

	var result string
	err := c.GetWithFallback(ctx, "missing", &result, fallback)
	require.ErrorIs(t, err, ErrMissing)

	// No sentinel should be stored.
	_, ok := p.store["missing"]
	require.False(t, ok, "sentinel should not be stored when negative caching is disabled")

	// Second call should re-invoke fallback.
	err = c.GetWithFallback(ctx, "missing", &result, fallback)
	require.ErrorIs(t, err, ErrMissing)
	require.Equal(t, 2, calls, "fallback called unexpected number of times")
}

func TestGetWithFallback_NegativeCaching_StoresSentinel(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithNegativeTtl(30*time.Second))
	ctx := t.Context()

	var result string
	err := c.GetWithFallback(ctx, "missing", &result, func() (any, time.Duration, error) {
		return (*string)(nil), TTLUseDefault, nil
	})
	require.ErrorIs(t, err, ErrMissing)

	// Sentinel should be stored with the negative TTL.
	stored, ok := p.store["missing"]
	require.True(t, ok, "sentinel not stored in provider")
	require.Len(t, stored, 1)
	require.Equal(t, byte(0x00), stored[0])
	require.Equal(t, 30*time.Second, p.lastTTL)
}

func TestGetWithFallback_NegativeCaching_ReturnsMissing(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithNegativeTtl(30*time.Second))
	ctx := t.Context()

	calls := 0
	fallback := func() (any, time.Duration, error) {
		calls++
		return (*string)(nil), TTLUseDefault, nil
	}

	var result string
	// First call stores sentinel.
	_ = c.GetWithFallback(ctx, "missing", &result, fallback)

	// Second call should return ErrMissing without invoking fallback.
	err := c.GetWithFallback(ctx, "missing", &result, fallback)
	require.ErrorIs(t, err, ErrMissing)
	require.Equal(t, 1, calls, "fallback called unexpected number of times")
}

func TestGet_NegativeEntry_ReturnsMissing(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithNegativeTtl(30*time.Second))
	ctx := t.Context()

	// Manually store a negative sentinel.
	p.store["neg-key"] = []byte{0x00}

	var result string
	err := c.Get(ctx, "neg-key", &result)
	require.ErrorIs(t, err, ErrMissing)
}

func TestExists_NegativeEntry_ReturnsFalse(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithNegativeTtl(30*time.Second))
	ctx := t.Context()

	// Manually store a negative sentinel.
	p.store["neg-key"] = []byte{0x00}

	exists, err := c.Exists(ctx, "neg-key")
	require.NoError(t, err)
	require.False(t, exists, "expected false for negative entry")
}

func TestDelete_ClearsNegativeEntry(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithNegativeTtl(30*time.Second))
	ctx := t.Context()

	calls := 0
	fallback := func() (any, time.Duration, error) {
		calls++
		return (*string)(nil), TTLUseDefault, nil
	}

	// Store a negative sentinel via fallback.
	var result string
	_ = c.GetWithFallback(ctx, "key1", &result, fallback)

	// Delete the entry.
	require.NoError(t, c.Delete(ctx, "key1"))

	// Next call should re-invoke fallback.
	_ = c.GetWithFallback(ctx, "key1", &result, fallback)
	require.Equal(t, 2, calls, "fallback called unexpected number of times")
}

func TestSave_OverwritesNegativeEntry(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithNegativeTtl(30*time.Second))
	ctx := t.Context()

	// Store a negative sentinel.
	p.store["key1"] = []byte{0x00}

	// Save a real value over it.
	require.NoError(t, c.Save(ctx, "key1", "real-value"))

	// Get should return the real value.
	var result string
	require.NoError(t, c.Get(ctx, "key1", &result))
	require.Equal(t, "real-value", result)
}

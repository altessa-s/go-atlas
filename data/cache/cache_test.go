// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"errors"
	"testing"
	"time"

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
	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.provider != p {
		t.Error("provider not set")
	}
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
	if c == nil {
		t.Fatal("NewNoop returned nil")
	}
	if _, ok := c.provider.(*noop.Provider); !ok {
		t.Error("expected noop provider")
	}
}

func TestSave(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	err := c.Save(ctx, "key1", "hello")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok := p.store["key1"]; !ok {
		t.Error("key not stored in provider")
	}
}

func TestGet_Hit(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	_ = c.Save(ctx, "key1", "hello")

	var result string
	err := c.Get(ctx, "key1", &result)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if result != "hello" {
		t.Errorf("got %q, want %q", result, "hello")
	}
}

func TestGet_Miss(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	var result string
	err := c.Get(ctx, "missing", &result)
	if !errors.Is(err, ErrMissing) {
		t.Errorf("expected ErrMissing, got %v", err)
	}
}

func TestExists(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	_ = c.Save(ctx, "key1", "hello")

	exists, err := c.Exists(ctx, "key1")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("expected true")
	}

	exists, err = c.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("expected false")
	}
}

func TestDelete(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	_ = c.Save(ctx, "key1", "hello")
	err := c.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var result string
	err = c.Get(ctx, "key1", &result)
	if !errors.Is(err, ErrMissing) {
		t.Errorf("expected ErrMissing after delete, got %v", err)
	}
}

func TestDeleteMany(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	_ = c.Save(ctx, "k1", "a")
	_ = c.Save(ctx, "k2", "b")

	err := c.DeleteMany(ctx, "k1", "k2")
	if err != nil {
		t.Fatalf("DeleteMany: %v", err)
	}

	if len(p.store) != 0 {
		t.Errorf("expected empty store, got %d items", len(p.store))
	}
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
	if err != nil {
		t.Fatalf("GetWithFallback: %v", err)
	}
	if result != "cached" {
		t.Errorf("got %q, want %q", result, "cached")
	}
}

func TestGetWithFallback_CacheMiss_FallbackHit(t *testing.T) {
	p := newMockProvider()
	c := New(p)
	ctx := t.Context()

	var result string
	err := c.GetWithFallback(ctx, "key1", &result, func() (any, time.Duration, error) {
		return "from-fallback", TTLUseDefault, nil
	})
	if err != nil {
		t.Fatalf("GetWithFallback: %v", err)
	}
	if result != "from-fallback" {
		t.Errorf("got %q, want %q", result, "from-fallback")
	}
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
	if !errors.Is(err, wantErr) {
		t.Errorf("got %v, want %v", err, wantErr)
	}
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

	if r1 != "value" || r2 != "value" {
		t.Errorf("unexpected results: %q, %q", r1, r2)
	}
	// The first call triggers fallback, second gets cache hit.
	if calls != 1 {
		t.Errorf("fallback called %d times, want 1", calls)
	}
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
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("expected ErrMissing, got %v", err)
	}

	// No sentinel should be stored.
	if _, ok := p.store["missing"]; ok {
		t.Error("sentinel should not be stored when negative caching is disabled")
	}

	// Second call should re-invoke fallback.
	err = c.GetWithFallback(ctx, "missing", &result, fallback)
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("expected ErrMissing, got %v", err)
	}
	if calls != 2 {
		t.Errorf("fallback called %d times, want 2", calls)
	}
}

func TestGetWithFallback_NegativeCaching_StoresSentinel(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithNegativeTtl(30*time.Second))
	ctx := t.Context()

	var result string
	err := c.GetWithFallback(ctx, "missing", &result, func() (any, time.Duration, error) {
		return (*string)(nil), TTLUseDefault, nil
	})
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("expected ErrMissing, got %v", err)
	}

	// Sentinel should be stored with the negative TTL.
	stored, ok := p.store["missing"]
	if !ok {
		t.Fatal("sentinel not stored in provider")
	}
	if len(stored) != 1 || stored[0] != 0x00 {
		t.Errorf("expected sentinel [0x00], got %v", stored)
	}
	if p.lastTTL != 30*time.Second {
		t.Errorf("expected negative TTL 30s, got %v", p.lastTTL)
	}
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
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("expected ErrMissing, got %v", err)
	}
	if calls != 1 {
		t.Errorf("fallback called %d times, want 1", calls)
	}
}

func TestGet_NegativeEntry_ReturnsMissing(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithNegativeTtl(30*time.Second))
	ctx := t.Context()

	// Manually store a negative sentinel.
	p.store["neg-key"] = []byte{0x00}

	var result string
	err := c.Get(ctx, "neg-key", &result)
	if !errors.Is(err, ErrMissing) {
		t.Errorf("expected ErrMissing, got %v", err)
	}
}

func TestExists_NegativeEntry_ReturnsFalse(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithNegativeTtl(30*time.Second))
	ctx := t.Context()

	// Manually store a negative sentinel.
	p.store["neg-key"] = []byte{0x00}

	exists, err := c.Exists(ctx, "neg-key")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("expected false for negative entry")
	}
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
	if err := c.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Next call should re-invoke fallback.
	_ = c.GetWithFallback(ctx, "key1", &result, fallback)
	if calls != 2 {
		t.Errorf("fallback called %d times, want 2", calls)
	}
}

func TestSave_OverwritesNegativeEntry(t *testing.T) {
	p := newMockProvider()
	c := New(p, WithNegativeTtl(30*time.Second))
	ctx := t.Context()

	// Store a negative sentinel.
	p.store["key1"] = []byte{0x00}

	// Save a real value over it.
	if err := c.Save(ctx, "key1", "real-value"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Get should return the real value.
	var result string
	if err := c.Get(ctx, "key1", &result); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if result != "real-value" {
		t.Errorf("got %q, want %q", result, "real-value")
	}
}

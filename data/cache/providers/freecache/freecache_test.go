// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package freecache_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/cache/providers"
	"github.com/altessa-s/go-atlas/data/cache/providers/freecache"
)

func TestNew_Default(t *testing.T) {
	p := freecache.New()
	if p == nil {
		t.Fatal("expected provider to be non-nil")
	}
}

func TestNew_WithMaxSize(t *testing.T) {
	p := freecache.New(freecache.WithMaxSize(1024 * 1024))
	if p == nil {
		t.Fatal("expected provider to be non-nil")
	}
}

func TestProvider_SaveAndGet(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := p.Save(ctx, key, value, 10*time.Second)
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	got, err := p.Get(ctx, key)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if string(got) != string(value) {
		t.Errorf("expected %q, got %q", value, got)
	}
}

func TestProvider_Get_Missing(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	_, err := p.Get(ctx, "nonexistent-key")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, providers.ErrMissing) {
		t.Errorf("expected providers.ErrMissing, got %v", err)
	}
}

func TestProvider_Exists_True(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := p.Save(ctx, key, value, 10*time.Second)
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	exists, err := p.Exists(ctx, key)
	if err != nil {
		t.Fatalf("failed to check exists: %v", err)
	}

	if !exists {
		t.Error("expected key to exist")
	}
}

func TestProvider_Exists_False(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	exists, err := p.Exists(ctx, "nonexistent-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exists {
		t.Error("expected key to not exist")
	}
}

func TestProvider_Delete(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := p.Save(ctx, key, value, 10*time.Second)
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	err = p.Delete(ctx, key)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	_, err = p.Get(ctx, key)
	if !errors.Is(err, providers.ErrMissing) {
		t.Errorf("expected providers.ErrMissing after delete, got %v", err)
	}
}

func TestProvider_DeleteMany(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	keys := []string{"key1", "key2", "key3"}
	value := []byte("test-value")

	for _, key := range keys {
		err := p.Save(ctx, key, value, 10*time.Second)
		if err != nil {
			t.Fatalf("failed to save key %s: %v", key, err)
		}
	}

	err := p.DeleteMany(ctx, keys...)
	if err != nil {
		t.Fatalf("failed to delete many: %v", err)
	}

	for _, key := range keys {
		_, err := p.Get(ctx, key)
		if !errors.Is(err, providers.ErrMissing) {
			t.Errorf("expected providers.ErrMissing for key %s after delete, got %v", key, err)
		}
	}
}

func TestProvider_SaveWithTTL(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := p.Save(ctx, key, value, 1*time.Second)
	if err != nil {
		t.Fatalf("failed to save with TTL: %v", err)
	}
}

func TestProvider_Concurrent(t *testing.T) {
	p := freecache.New()
	ctx := t.Context()

	const numGoroutines = 100
	var wg sync.WaitGroup

	for i := range numGoroutines {
		wg.Go(func() {
			key := string(rune('a' + (i % 26)))
			value := []byte("value-" + string(rune('0'+(i%10))))
			err := p.Save(ctx, key, value, 10*time.Second)
			if err != nil {
				t.Errorf("failed to save concurrently: %v", err)
			}
		})

		wg.Go(func() {
			key := string(rune('a' + (i % 26)))
			_, _ = p.Get(ctx, key)
		})
	}

	wg.Wait()
}

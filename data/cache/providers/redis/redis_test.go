// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/altessa-s/go-atlas/data/cache/providers"
	"github.com/altessa-s/go-atlas/data/cache/providers/redis"

	goredis "github.com/redis/go-redis/v9"
)

// setupProvider creates a Provider with a miniredis instance for testing.
func setupProvider(t *testing.T) (*redis.Provider, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	t.Cleanup(func() {
		mr.Close()
	})

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	provider := redis.New(client)
	return provider, mr
}

func TestNew(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	provider := redis.New(client)
	if provider == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithPrefix(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	provider := redis.New(client, redis.WithPrefix("test:"))
	if provider == nil {
		t.Fatal("New() with prefix returned nil")
	}
}

func TestProvider_SaveAndGet(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := provider.Save(ctx, key, value, 0)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	got, err := provider.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if string(got) != string(value) {
		t.Errorf("Get() = %q, want %q", got, value)
	}
}

func TestProvider_Get_Missing(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	_, err := provider.Get(ctx, "nonexistent-key")
	if !errors.Is(err, providers.ErrMissing) {
		t.Errorf("Get() error = %v, want providers.ErrMissing", err)
	}
}

func TestProvider_Exists_True(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := provider.Save(ctx, key, value, 0)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	exists, err := provider.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() failed: %v", err)
	}

	if !exists {
		t.Error("Exists() = false, want true")
	}
}

func TestProvider_Exists_False(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	exists, err := provider.Exists(ctx, "nonexistent-key")
	if err != nil {
		t.Fatalf("Exists() failed: %v", err)
	}

	if exists {
		t.Error("Exists() = true, want false")
	}
}

func TestProvider_Delete(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := provider.Save(ctx, key, value, 0)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	err = provider.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	_, err = provider.Get(ctx, key)
	if !errors.Is(err, providers.ErrMissing) {
		t.Errorf("Get() after Delete() error = %v, want providers.ErrMissing", err)
	}
}

func TestProvider_DeleteMany(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	keys := []string{"key1", "key2", "key3"}
	value := []byte("test-value")

	for _, key := range keys {
		err := provider.Save(ctx, key, value, 0)
		if err != nil {
			t.Fatalf("Save() failed for key %s: %v", key, err)
		}
	}

	err := provider.DeleteMany(ctx, keys...)
	if err != nil {
		t.Fatalf("DeleteMany() failed: %v", err)
	}

	for _, key := range keys {
		_, err := provider.Get(ctx, key)
		if !errors.Is(err, providers.ErrMissing) {
			t.Errorf("Get() after DeleteMany() for key %s error = %v, want providers.ErrMissing", key, err)
		}
	}
}

func TestProvider_SaveWithTTL(t *testing.T) {
	provider, mr := setupProvider(t)
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")
	ttl := 10 * time.Second

	err := provider.Save(ctx, key, value, ttl)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify key exists before TTL expires
	got, err := provider.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() before TTL failed: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("Get() before TTL = %q, want %q", got, value)
	}

	// Fast-forward time in miniredis
	mr.FastForward(15 * time.Second)

	// Verify key is expired
	_, err = provider.Get(ctx, key)
	if !errors.Is(err, providers.ErrMissing) {
		t.Errorf("Get() after TTL error = %v, want providers.ErrMissing", err)
	}
}

func TestProvider_WithPrefix_Isolation(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	provider1 := redis.New(client, redis.WithPrefix("prefix1:"))
	provider2 := redis.New(client, redis.WithPrefix("prefix2:"))

	ctx := t.Context()
	key := "shared-key"
	value1 := []byte("value-1")
	value2 := []byte("value-2")

	// Save to provider1
	err := provider1.Save(ctx, key, value1, 0)
	if err != nil {
		t.Fatalf("provider1.Save() failed: %v", err)
	}

	// Save to provider2 with same key
	err = provider2.Save(ctx, key, value2, 0)
	if err != nil {
		t.Fatalf("provider2.Save() failed: %v", err)
	}

	// Verify provider1 still has its value
	got1, err := provider1.Get(ctx, key)
	if err != nil {
		t.Fatalf("provider1.Get() failed: %v", err)
	}
	if string(got1) != string(value1) {
		t.Errorf("provider1.Get() = %q, want %q", got1, value1)
	}

	// Verify provider2 has its value
	got2, err := provider2.Get(ctx, key)
	if err != nil {
		t.Fatalf("provider2.Get() failed: %v", err)
	}
	if string(got2) != string(value2) {
		t.Errorf("provider2.Get() = %q, want %q", got2, value2)
	}

	// Verify values are different
	if string(got1) == string(got2) {
		t.Error("provider1 and provider2 values should be different but are the same")
	}
}

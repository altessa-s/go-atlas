// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"context"
	"errors"
	"testing"
)

func TestNewCache_InvalidSize(t *testing.T) {
	_, err := NewCache[string, int](0)
	if err == nil {
		t.Error("NewCache(0) should return error")
	}
}

func TestNewShardedCache_SmallSize(t *testing.T) {
	// Size 0 still works (shardSize becomes 1)
	c, err := NewShardedCache[string, int](0)
	if err != nil {
		t.Fatalf("NewShardedCache(0) error = %v", err)
	}
	c.Put("a", 1)
	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Errorf("got (%d, %v), want (1, true)", v, ok)
	}
}

func TestNewShardedCache_WithShardCount(t *testing.T) {
	c, err := NewShardedCache[string, int](100, WithShardCount(32))
	if err != nil {
		t.Fatal(err)
	}
	c.Put("a", 1)
	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Errorf("got (%d, %v), want (1, true)", v, ok)
	}
}

func TestShardedCache_Keys(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	keys := make(map[string]bool)
	for k := range c.Keys() {
		keys[k] = true
	}
	if len(keys) != 3 {
		t.Errorf("Keys() returned %d keys, want 3", len(keys))
	}
}

func TestShardedCache_All(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("x", 10)
	c.Put("y", 20)

	pairs := make(map[string]int)
	for k, v := range c.All() {
		pairs[k] = v
	}
	if pairs["x"] != 10 || pairs["y"] != 20 {
		t.Errorf("All: %v", pairs)
	}
}

func TestShardedCache_GetOrCompute_Miss(t *testing.T) {
	c, _ := NewShardedCache[string, string](100)
	ctx := t.Context()

	v, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		return "computed", nil
	})
	if err != nil || v != "computed" {
		t.Errorf("got (%q, %v), want (computed, nil)", v, err)
	}

	v2, ok := c.Get("key")
	if !ok || v2 != "computed" {
		t.Error("value not cached after GetOrCompute")
	}
}

func TestShardedCache_GetOrCompute_Hit(t *testing.T) {
	c, _ := NewShardedCache[string, string](100)
	c.Put("key", "existing")
	ctx := t.Context()

	v, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		t.Error("compute should not be called on hit")
		return "", nil
	})
	if err != nil || v != "existing" {
		t.Errorf("got (%q, %v)", v, err)
	}
}

func TestShardedCache_GetOrCompute_Error(t *testing.T) {
	c, _ := NewShardedCache[string, string](100)
	ctx := t.Context()

	wantErr := errors.New("fail")
	_, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("got %v, want %v", err, wantErr)
	}
}

func TestShardedCache_Has(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("a", 1)
	if !c.Has("a") {
		t.Error("Has returned false for existing key")
	}
	if c.Has("b") {
		t.Error("Has returned true for missing key")
	}
}

func TestShardedCache_IntKey(t *testing.T) {
	c, _ := NewShardedCache[int, string](100)
	c.Put(42, "hello")
	v, ok := c.Get(42)
	if !ok || v != "hello" {
		t.Errorf("int key: got (%q, %v)", v, ok)
	}
}

func TestShardedCache_Int64Key(t *testing.T) {
	c, _ := NewShardedCache[int64, string](100)
	c.Put(int64(99), "world")
	v, ok := c.Get(int64(99))
	if !ok || v != "world" {
		t.Errorf("int64 key: got (%q, %v)", v, ok)
	}
}

func TestShardedCache_Uint64Key(t *testing.T) {
	c, _ := NewShardedCache[uint64, string](100)
	c.Put(uint64(77), "test")
	v, ok := c.Get(uint64(77))
	if !ok || v != "test" {
		t.Errorf("uint64 key: got (%q, %v)", v, ok)
	}
}

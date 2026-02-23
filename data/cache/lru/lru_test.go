// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"context"
	"errors"
	"testing"
)

func TestCache_PutGet(t *testing.T) {
	c, err := NewCache[string, int](10)
	if err != nil {
		t.Fatal(err)
	}
	c.Put("a", 1)
	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Errorf("got (%d, %v), want (1, true)", v, ok)
	}
}

func TestCache_Remove(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	if !c.Remove("a") {
		t.Error("Remove returned false for existing key")
	}
	if _, ok := c.Get("a"); ok {
		t.Error("key still present after remove")
	}
}

func TestCache_Has(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	if !c.Has("a") {
		t.Error("Has returned false for existing key")
	}
	if c.Has("b") {
		t.Error("Has returned true for missing key")
	}
}

func TestCache_Len(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	c.Put("b", 2)
	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2", c.Len())
	}
}

func TestCache_Purge(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Purge()
	if c.Len() != 0 {
		t.Errorf("Len after Purge = %d, want 0", c.Len())
	}
}

func TestCache_Keys(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	c.Put("b", 2)

	keys := make(map[string]bool)
	for k := range c.Keys() {
		keys[k] = true
	}
	if !keys["a"] || !keys["b"] {
		t.Errorf("Keys missing expected entries: %v", keys)
	}
}

func TestCache_All(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	c.Put("b", 2)

	pairs := make(map[string]int)
	for k, v := range c.All() {
		pairs[k] = v
	}
	if pairs["a"] != 1 || pairs["b"] != 2 {
		t.Errorf("All: %v", pairs)
	}
}

func TestCache_GetOrCompute_Miss(t *testing.T) {
	c, _ := NewCache[string, string](10)
	ctx := t.Context()

	v, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		return "computed", nil
	})
	if err != nil || v != "computed" {
		t.Errorf("got (%q, %v), want (computed, nil)", v, err)
	}

	// Should now be cached.
	v2, ok := c.Get("key")
	if !ok || v2 != "computed" {
		t.Error("value not cached after GetOrCompute")
	}
}

func TestCache_GetOrCompute_Hit(t *testing.T) {
	c, _ := NewCache[string, string](10)
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

func TestCache_Eviction(t *testing.T) {
	c, _ := NewCache[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // should evict "a"

	if c.Has("a") {
		t.Error("expected 'a' to be evicted")
	}
	if !c.Has("b") || !c.Has("c") {
		t.Error("expected 'b' and 'c' to still exist")
	}
}

func TestCache_GetOrCompute_Error(t *testing.T) {
	c, _ := NewCache[string, string](10)
	ctx := t.Context()

	wantErr := errors.New("fail")
	_, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("got %v, want %v", err, wantErr)
	}
}

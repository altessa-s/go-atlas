// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"sync"
	"testing"
)

func TestShardedCache_PutGet(t *testing.T) {
	c, err := NewShardedCache[string, int](100)
	if err != nil {
		t.Fatal(err)
	}
	c.Put("a", 1)
	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Errorf("got (%d, %v), want (1, true)", v, ok)
	}
}

func TestShardedCache_Remove(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("a", 1)
	if !c.Remove("a") {
		t.Error("Remove returned false")
	}
	if _, ok := c.Get("a"); ok {
		t.Error("key still present after remove")
	}
}

func TestShardedCache_Len(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)
	if c.Len() != 3 {
		t.Errorf("Len = %d, want 3", c.Len())
	}
}

func TestShardedCache_Purge(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Purge()
	if c.Len() != 0 {
		t.Errorf("Len after Purge = %d, want 0", c.Len())
	}
}

func TestShardedCache_Concurrent(t *testing.T) {
	c, _ := NewShardedCache[int, int](1000)
	var wg sync.WaitGroup
	n := 100

	for i := range n {
		wg.Go(func() {
			c.Put(i, i*10)
			c.Get(i)
			c.Has(i)
		})
	}
	wg.Wait()

	if c.Len() == 0 {
		t.Error("expected items in cache after concurrent puts")
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"sync"
	"testing"
	"time"
)

func TestMemory_AttemptLock_New(t *testing.T) {
	s := New()
	ctx := t.Context()

	ok, existing, err := s.AttemptLock(ctx, "key1", []byte("val"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected lock acquired")
	}
	if existing != nil {
		t.Error("expected nil existing value")
	}
}

func TestMemory_AttemptLock_Existing(t *testing.T) {
	s := New()
	ctx := t.Context()

	_, _, _ = s.AttemptLock(ctx, "key1", []byte("first"))

	ok, existing, err := s.AttemptLock(ctx, "key1", []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected lock NOT acquired")
	}
	if string(existing) != "first" {
		t.Errorf("existing = %q, want %q", existing, "first")
	}
}

func TestMemory_Complete(t *testing.T) {
	s := New()
	ctx := t.Context()

	_, _, _ = s.AttemptLock(ctx, "key1", []byte("in-progress"))
	err := s.Complete(ctx, "key1", []byte("done"))
	if err != nil {
		t.Fatal(err)
	}

	// Attempting lock should now return the completed value.
	ok, existing, _ := s.AttemptLock(ctx, "key1", []byte("new"))
	if ok {
		t.Error("expected lock NOT acquired after complete")
	}
	if string(existing) != "done" {
		t.Errorf("existing = %q, want %q", existing, "done")
	}
}

func TestMemory_Delete(t *testing.T) {
	s := New()
	ctx := t.Context()

	_, _, _ = s.AttemptLock(ctx, "key1", []byte("val"))
	err := s.Delete(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}

	// After delete, lock should succeed.
	ok, _, _ := s.AttemptLock(ctx, "key1", []byte("val2"))
	if !ok {
		t.Error("expected lock acquired after delete")
	}
}

func TestMemory_TTLExpiry(t *testing.T) {
	s := New(WithTtl(10 * time.Millisecond))
	ctx := t.Context()

	_, _, _ = s.AttemptLock(ctx, "key1", []byte("val"))
	time.Sleep(20 * time.Millisecond)

	// Expired entry should be treated as non-existent.
	ok, _, err := s.AttemptLock(ctx, "key1", []byte("val2"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected lock acquired after TTL expiry")
	}
}

func TestMemory_Concurrent(t *testing.T) {
	s := New()
	ctx := t.Context()
	var wg sync.WaitGroup
	n := 100
	acquired := make(chan bool, n)

	for range n {
		wg.Go(func() {
			ok, _, _ := s.AttemptLock(ctx, "shared-key", []byte("val"))
			acquired <- ok
		})
	}
	wg.Wait()
	close(acquired)

	count := 0
	for ok := range acquired {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 lock acquired, got %d", count)
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages/memory"
)

func TestProvider_Allow_UnderLimit(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	info, err := p.Allow(ctx, "key1", 5, time.Minute)
	if err != nil {
		t.Fatalf("Allow() error: %v", err)
	}
	if info.Remaining != 4 {
		t.Errorf("Remaining = %d, want 4", info.Remaining)
	}
}

func TestProvider_Allow_ExceedsLimit(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	for range 3 {
		_, err := p.Allow(ctx, "key1", 3, time.Minute)
		if err != nil {
			t.Fatalf("Allow() error: %v", err)
		}
	}

	info, err := p.Allow(ctx, "key1", 3, time.Minute)
	if !errors.Is(err, storages.ErrLimitExceeded) {
		t.Errorf("Allow() error = %v, want ErrLimitExceeded", err)
	}
	if info.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", info.Remaining)
	}
}

func TestProvider_Allow_DifferentKeys(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	_, err := p.Allow(ctx, "key1", 1, time.Minute)
	if err != nil {
		t.Fatalf("Allow(key1) error: %v", err)
	}

	_, err = p.Allow(ctx, "key2", 1, time.Minute)
	if err != nil {
		t.Fatalf("Allow(key2) should succeed independently: %v", err)
	}
}

func TestProvider_Reset(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	// Exhaust the limit
	for range 3 {
		_, _ = p.Allow(ctx, "key1", 3, time.Minute)
	}

	err := p.Reset(ctx, "key1")
	if err != nil {
		t.Fatalf("Reset() error: %v", err)
	}

	// Should be allowed again
	_, err = p.Allow(ctx, "key1", 3, time.Minute)
	if err != nil {
		t.Errorf("Allow() after Reset() should succeed: %v", err)
	}
}

func TestProvider_Reset_NonExistent(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	err := p.Reset(ctx, "nonexistent")
	if err != nil {
		t.Errorf("Reset() on nonexistent key should not error: %v", err)
	}
}

func TestProvider_Close(t *testing.T) {
	p := memory.New()
	if err := p.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestProvider_RunCleanup(t *testing.T) {
	p := memory.New(memory.WithMaxIdleTime(1 * time.Millisecond))
	ctx := t.Context()

	_, _ = p.Allow(ctx, "key1", 10, time.Minute)

	time.Sleep(5 * time.Millisecond)
	p.RunCleanup()

	// After cleanup, key should be gone; new request should succeed
	info, err := p.Allow(ctx, "key1", 10, time.Minute)
	if err != nil {
		t.Fatalf("Allow() after cleanup should succeed: %v", err)
	}
	if info.Remaining != 9 {
		t.Errorf("Remaining = %d, want 9 (fresh bucket)", info.Remaining)
	}
}

func TestProvider_Allow_Panics(t *testing.T) {
	p := memory.New()
	ctx := t.Context()

	tests := []struct {
		name string
		fn   func()
	}{
		{"nil context", func() { p.Allow(nil, "k", 1, time.Second) }}, //nolint:staticcheck,SA1012
		{"empty key", func() { p.Allow(ctx, "", 1, time.Second) }},
		{"zero limit", func() { p.Allow(ctx, "k", 0, time.Second) }},
		{"zero period", func() { p.Allow(ctx, "k", 1, 0) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected panic")
				}
			}()
			tt.fn()
		})
	}
}

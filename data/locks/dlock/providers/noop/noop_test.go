// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/noop"
)

func TestNew(t *testing.T) {
	p := noop.New()
	if p == nil {
		t.Fatal("New() returned nil")
	}
}

func TestProvider_Lock(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	lk, err := p.Lock(ctx, "test-key")
	if err != nil {
		t.Fatalf("Lock() error: %v", err)
	}
	if lk == nil {
		t.Fatal("Lock() returned nil")
	}

	info, err := lk.GetLockInfo(ctx)
	if err != nil {
		t.Fatalf("GetLockInfo() error: %v", err)
	}
	if info.Key != "test-key" {
		t.Errorf("Key = %q, want %q", info.Key, "test-key")
	}
	if info.Owner != "nop" {
		t.Errorf("Owner = %q, want %q", info.Owner, "nop")
	}
}

func TestProvider_Lock_AfterClose(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	_ = p.Close(ctx)

	_, err := p.Lock(ctx, "test-key")
	if err == nil {
		t.Error("Lock() after Close() should return error")
	}
}

func TestProvider_GetLockInfo(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	info, err := p.GetLockInfo(ctx, "any-key")
	if err != nil {
		t.Fatalf("GetLockInfo() error: %v", err)
	}
	if info.Key != "any-key" {
		t.Errorf("Key = %q, want %q", info.Key, "any-key")
	}
	if info.Owner != "nop" {
		t.Errorf("Owner = %q, want %q", info.Owner, "nop")
	}
	if info.IsStale {
		t.Error("IsStale should be false")
	}
	if info.FencingToken != 0 {
		t.Errorf("FencingToken = %d, want 0 for noop provider", info.FencingToken)
	}
}

func TestLock_Release(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	lk, _ := p.Lock(ctx, "key")
	if err := lk.Release(ctx); err != nil {
		t.Errorf("Release() error: %v", err)
	}
}

func TestLock_Release_CanceledContext(t *testing.T) {
	p := noop.New()

	lk, _ := p.Lock(t.Context(), "key")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := lk.Release(ctx); err == nil {
		t.Error("Release() with canceled context should return error")
	}
}

func TestProvider_Close(t *testing.T) {
	p := noop.New()
	if err := p.Close(t.Context()); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestProvider_Lock_DifferentKeys(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	keys := []string{"key-a", "key-b", "key-c"}
	for _, key := range keys {
		lk, err := p.Lock(ctx, key)
		if err != nil {
			t.Fatalf("Lock(%q) error: %v", key, err)
		}
		info, _ := lk.GetLockInfo(ctx)
		if info.Key != key {
			t.Errorf("Lock(%q): info.Key = %q", key, info.Key)
		}
		_ = lk.Release(ctx)
	}
}

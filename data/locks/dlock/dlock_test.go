// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/locks/dlock"
)

func TestNewWithNoop(t *testing.T) {
	dl := dlock.NewWithNoop()
	if dl == nil {
		t.Fatal("NewWithNoop() returned nil")
	}
}

func TestDLock_Lock_EmptyKey(t *testing.T) {
	dl := dlock.NewWithNoop()
	_, err := dl.Lock(t.Context(), "")
	if err == nil {
		t.Error("Lock(\"\") should return error")
	}
}

func TestDLock_Lock_Success(t *testing.T) {
	dl := dlock.NewWithNoop()
	ctx := t.Context()

	lk, err := dl.Lock(ctx, "test-key")
	if err != nil {
		t.Fatalf("Lock() error: %v", err)
	}
	if lk == nil {
		t.Fatal("Lock() returned nil")
	}
	if err := lk.Release(ctx); err != nil {
		t.Errorf("Release() error: %v", err)
	}
}

func TestDLock_Synchronize_Success(t *testing.T) {
	dl := dlock.NewWithNoop()
	ctx := t.Context()

	called := false
	err := dl.Synchronize(ctx, "test-key", func(ctx context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("Synchronize() error: %v", err)
	}
	if !called {
		t.Error("fn was not called")
	}
}

func TestDLock_Synchronize_EmptyKey(t *testing.T) {
	dl := dlock.NewWithNoop()
	err := dl.Synchronize(t.Context(), "", func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("Synchronize(\"\") should return error")
	}
}

func TestDLock_Synchronize_FnError(t *testing.T) {
	dl := dlock.NewWithNoop()
	ctx := t.Context()

	wantErr := errors.New("fn error")
	err := dl.Synchronize(ctx, "key", func(ctx context.Context) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("Synchronize() error = %v, want %v", err, wantErr)
	}
}

func TestDLock_GetLockInfo_EmptyKey(t *testing.T) {
	dl := dlock.NewWithNoop()
	_, err := dl.GetLockInfo(t.Context(), "")
	if err == nil {
		t.Error("GetLockInfo(\"\") should return error")
	}
}

func TestDLock_GetLockInfo_Success(t *testing.T) {
	dl := dlock.NewWithNoop()
	ctx := t.Context()

	info, err := dl.GetLockInfo(ctx, "test-key")
	if err != nil {
		t.Fatalf("GetLockInfo() error: %v", err)
	}
	if info.Key != "test-key" {
		t.Errorf("Key = %q, want %q", info.Key, "test-key")
	}
	if info.FencingToken != 0 {
		t.Errorf("FencingToken = %d, want 0 for noop-backed dlock", info.FencingToken)
	}
}

func TestDLock_Close(t *testing.T) {
	dl := dlock.NewWithNoop()
	if err := dl.Close(t.Context()); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestDLock_WithLockAcquireTimeout(t *testing.T) {
	dl := dlock.NewWithNoop(dlock.WithLockAcquireTimeout(5 * time.Second))
	ctx := t.Context()

	err := dl.Synchronize(ctx, "key", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Synchronize() error: %v", err)
	}
}

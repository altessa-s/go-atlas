// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
)

func setupLocker(tb testing.TB) *locknats.Locker {
	tb.Helper()
	ns := testhelpers.StartNATSServer(tb)
	nc := testhelpers.ConnectNATS(tb, ns)
	ctx := tb.Context()

	bucket := strings.ReplaceAll(tb.Name(), "/", "-")
	locker, err := locknats.New(ctx, nc, locknats.WithBucket(bucket))
	if err != nil {
		tb.Fatalf("failed to create NATS locker: %v", err)
	}
	tb.Cleanup(func() { _ = locker.Close(context.Background()) })
	return locker
}

func TestNew(t *testing.T) {
	locker := setupLocker(t)
	if locker == nil {
		t.Fatal("New() returned nil")
	}
}

func TestLocker_Lock_Release(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	lk, err := locker.Lock(ctx, "test-key")
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

	if err := lk.Release(ctx); err != nil {
		t.Errorf("Release() error: %v", err)
	}
}

func TestLocker_GetLockInfo_NotHeld(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	_, err := locker.GetLockInfo(ctx, "nonexistent")
	if err == nil {
		t.Error("GetLockInfo() on nonexistent key should return error")
	}
	if !isErrLockNotHeld(err) {
		t.Errorf("GetLockInfo() error = %v, want ErrLockNotHeld", err)
	}
}

func TestLocker_GetLockInfo_Held(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	lk, err := locker.Lock(ctx, "info-key")
	if err != nil {
		t.Fatalf("Lock() error: %v", err)
	}
	defer lk.Release(ctx) //nolint:errcheck

	info, err := locker.GetLockInfo(ctx, "info-key")
	if err != nil {
		t.Fatalf("GetLockInfo() error: %v", err)
	}
	if info.Key != "info-key" {
		t.Errorf("Key = %q, want %q", info.Key, "info-key")
	}
	if info.Owner == "" {
		t.Error("Owner should not be empty")
	}
	if info.AcquiredAt.IsZero() {
		t.Error("AcquiredAt should not be zero")
	}
	if info.FencingToken == 0 {
		t.Error("FencingToken should be > 0 for a held lock")
	}
}

func TestLocker_Close_PreventsNewLocks(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	if err := locker.Close(ctx); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	_, err := locker.Lock(ctx, "key")
	if err == nil {
		t.Error("Lock() after Close() should return error")
	}
}

func TestLocker_Close_Double(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	_ = locker.Close(ctx)
	err := locker.Close(ctx)
	if err == nil {
		t.Error("second Close() should return error")
	}
}

func TestLocker_Lock_CanceledContext(t *testing.T) {
	locker := setupLocker(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := locker.Lock(ctx, "key")
	if err == nil {
		t.Error("Lock() with canceled context should return error")
	}
}

func TestLocker_Lock_DifferentKeys(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	lk1, err := locker.Lock(ctx, "key-1")
	if err != nil {
		t.Fatalf("Lock(key-1) error: %v", err)
	}
	defer lk1.Release(ctx) //nolint:errcheck

	lk2, err := locker.Lock(ctx, "key-2")
	if err != nil {
		t.Fatalf("Lock(key-2) error: %v", err)
	}
	defer lk2.Release(ctx) //nolint:errcheck
}

func TestLocker_Close_ReleasesActiveLocks(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	_, err := locker.Lock(ctx, "active-key")
	if err != nil {
		t.Fatalf("Lock() error: %v", err)
	}

	// Close should release active locks without error
	if err := locker.Close(ctx); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestLocker_Lock_WithTimeout(t *testing.T) {
	locker := setupLocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	lk, err := locker.Lock(ctx, "timeout-key")
	if err != nil {
		t.Fatalf("Lock() error: %v", err)
	}
	_ = lk.Release(ctx)
}

func isErrLockNotHeld(err error) bool {
	return err != nil && err.Error() == errs.ErrLockNotHeld.Error()
}

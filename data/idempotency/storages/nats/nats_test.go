// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idempnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
)

func setupStorage(tb testing.TB) *idempnats.Storage {
	tb.Helper()
	ns := testhelpers.StartNATSServer(tb)
	_, js := testhelpers.ConnectJetStream(tb, ns)

	storage, err := idempnats.New(js, idempnats.WithBucket(tb.Name()))
	if err != nil {
		tb.Fatalf("failed to create NATS storage: %v", err)
	}
	return storage
}

func TestNew(t *testing.T) {
	storage := setupStorage(t)
	if storage == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_NilJetStream(t *testing.T) {
	_, err := idempnats.New(nil)
	if err == nil {
		t.Error("New(nil) should return error")
	}
}

func TestStorage_AttemptLock_New(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, existingVal, err := storage.AttemptLock(ctx, "key1", []byte("val1"))
	if err != nil {
		t.Fatalf("AttemptLock() error: %v", err)
	}
	if !locked {
		t.Error("expected lock to succeed on new key")
	}
	if existingVal != nil {
		t.Errorf("expected nil existing value, got: %v", existingVal)
	}
}

func TestStorage_AttemptLock_Existing(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, _, err := storage.AttemptLock(ctx, "key1", []byte("val1"))
	if err != nil {
		t.Fatalf("first AttemptLock() error: %v", err)
	}
	if !locked {
		t.Fatal("expected first lock to succeed")
	}

	locked, existingVal, err := storage.AttemptLock(ctx, "key1", []byte("val2"))
	if err != nil {
		t.Fatalf("second AttemptLock() error: %v", err)
	}
	if locked {
		t.Error("expected second lock to fail")
	}
	if string(existingVal) != "val1" {
		t.Errorf("expected existing value %q, got %q", "val1", existingVal)
	}
}

func TestStorage_AttemptLock_EmptyKey(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, existingVal, err := storage.AttemptLock(ctx, "", []byte("val"))
	if err != nil {
		t.Fatalf("AttemptLock(\"\") error: %v", err)
	}
	if !locked {
		t.Error("expected empty key to return locked=true")
	}
	if existingVal != nil {
		t.Errorf("expected nil existing value, got: %v", existingVal)
	}
}

func TestStorage_Complete(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, _, err := storage.AttemptLock(ctx, "key1", []byte("lock-val"))
	if err != nil {
		t.Fatalf("AttemptLock() error: %v", err)
	}
	if !locked {
		t.Fatal("expected lock to succeed")
	}

	if err := storage.Complete(ctx, "key1", []byte("complete-val")); err != nil {
		t.Fatalf("Complete() error: %v", err)
	}

	locked, existingVal, err := storage.AttemptLock(ctx, "key1", []byte("new-val"))
	if err != nil {
		t.Fatalf("AttemptLock() after Complete() error: %v", err)
	}
	if locked {
		t.Error("expected lock to fail after completion")
	}
	if string(existingVal) != "complete-val" {
		t.Errorf("expected completed value %q, got %q", "complete-val", existingVal)
	}
}

func TestStorage_Complete_EmptyKey(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	if err := storage.Complete(ctx, "", []byte("val")); err != nil {
		t.Errorf("Complete(\"\") should not error: %v", err)
	}
}

func TestStorage_Delete(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, _, err := storage.AttemptLock(ctx, "key1", []byte("val"))
	if err != nil {
		t.Fatalf("AttemptLock() error: %v", err)
	}
	if !locked {
		t.Fatal("expected lock to succeed")
	}

	if err := storage.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	locked, existingVal, err := storage.AttemptLock(ctx, "key1", []byte("val2"))
	if err != nil {
		t.Fatalf("AttemptLock() after Delete() error: %v", err)
	}
	if !locked {
		t.Error("expected lock to succeed after deletion")
	}
	if existingVal != nil {
		t.Errorf("expected nil existing value after deletion, got: %v", existingVal)
	}
}

func TestStorage_Delete_EmptyKey(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	if err := storage.Delete(ctx, ""); err != nil {
		t.Errorf("Delete(\"\") should not error: %v", err)
	}
}

func TestStorage_FullLifecycle(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	// Lock
	locked, _, err := storage.AttemptLock(ctx, "lifecycle", []byte("in-progress"))
	if err != nil {
		t.Fatalf("AttemptLock() error: %v", err)
	}
	if !locked {
		t.Fatal("expected lock to succeed")
	}

	// Complete
	if err := storage.Complete(ctx, "lifecycle", []byte("done")); err != nil {
		t.Fatalf("Complete() error: %v", err)
	}

	// Verify completed value
	locked, val, err := storage.AttemptLock(ctx, "lifecycle", []byte("retry"))
	if err != nil {
		t.Fatalf("AttemptLock() error: %v", err)
	}
	if locked {
		t.Error("expected lock to fail")
	}
	if string(val) != "done" {
		t.Errorf("expected %q, got %q", "done", val)
	}

	// Delete
	if err := storage.Delete(ctx, "lifecycle"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// Should be able to lock again
	locked, _, err = storage.AttemptLock(ctx, "lifecycle", []byte("new"))
	if err != nil {
		t.Fatalf("AttemptLock() after delete error: %v", err)
	}
	if !locked {
		t.Error("expected lock to succeed after deletion")
	}
}

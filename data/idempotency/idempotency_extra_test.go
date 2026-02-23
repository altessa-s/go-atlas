// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestNew_WithOptions(t *testing.T) {
	storage := testhelpers.NewMockIdempotencyStorage()
	keeper := idempotency.New(storage, idempotency.WithLogger(slog.Default()))
	if keeper == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithSerializer(t *testing.T) {
	storage := testhelpers.NewMockIdempotencyStorage()
	keeper := idempotency.New(storage, idempotency.WithSerializer(nil))
	if keeper == nil {
		t.Fatal("New() returned nil")
	}
}

func TestComplete_EmptyKey(t *testing.T) {
	storage := testhelpers.NewMockIdempotencyStorage()
	keeper := idempotency.New(storage)

	err := keeper.Complete(t.Context(), "", "data")
	if err != nil {
		t.Errorf("Complete('') error = %v", err)
	}
}

func TestDelete_EmptyKey(t *testing.T) {
	storage := testhelpers.NewMockIdempotencyStorage()
	keeper := idempotency.New(storage)

	err := keeper.Delete(t.Context(), "")
	if err != nil {
		t.Errorf("Delete('') error = %v", err)
	}
}

func TestStorageFunc_AllMethods(t *testing.T) {
	var lockCalled, completeCalled, deleteCalled bool

	sf := idempotency.StorageFunc{
		AttemptLockFunc: func(ctx context.Context, key string, val []byte) (bool, []byte, error) {
			lockCalled = true
			return true, nil, nil
		},
		CompleteFunc: func(ctx context.Context, key string, val []byte) error {
			completeCalled = true
			return nil
		},
		DeleteFunc: func(ctx context.Context, key string) error {
			deleteCalled = true
			return nil
		},
	}

	ctx := t.Context()
	_, _, _ = sf.AttemptLock(ctx, "k", nil)
	_ = sf.Complete(ctx, "k", nil)
	_ = sf.Delete(ctx, "k")

	if !lockCalled {
		t.Error("AttemptLockFunc not called")
	}
	if !completeCalled {
		t.Error("CompleteFunc not called")
	}
	if !deleteCalled {
		t.Error("DeleteFunc not called")
	}
}

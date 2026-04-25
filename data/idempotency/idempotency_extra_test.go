// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestNew_WithOptions(t *testing.T) {
	storage := testhelpers.NewMockIdempotencyStorage()
	keeper := idempotency.New(storage, idempotency.WithLogger(slog.Default()))
	require.NotNil(t, keeper)
}

func TestNew_WithSerializer(t *testing.T) {
	storage := testhelpers.NewMockIdempotencyStorage()
	keeper := idempotency.New(storage, idempotency.WithSerializer(nil))
	require.NotNil(t, keeper)
}

func TestComplete_EmptyKey(t *testing.T) {
	storage := testhelpers.NewMockIdempotencyStorage()
	keeper := idempotency.New(storage)

	err := keeper.Complete(t.Context(), "", "data")
	require.ErrorIs(t, err, idempotency.ErrEmptyKey)
}

func TestDelete_EmptyKey(t *testing.T) {
	storage := testhelpers.NewMockIdempotencyStorage()
	keeper := idempotency.New(storage)

	err := keeper.Delete(t.Context(), "")
	require.ErrorIs(t, err, idempotency.ErrEmptyKey)
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

	require.True(t, lockCalled, "AttemptLockFunc not called")
	require.True(t, completeCalled, "CompleteFunc not called")
	require.True(t, deleteCalled, "DeleteFunc not called")
}

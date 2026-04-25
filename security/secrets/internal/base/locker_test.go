// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets/internal/base"
)

type mockLocker struct {
	recordedKey string
	err         error
}

func (m *mockLocker) Synchronize(ctx context.Context, key string, fn func(ctx context.Context) error) error {
	m.recordedKey = key
	if m.err != nil {
		return m.err
	}
	return fn(ctx)
}

func TestWithLock_NilLocker(t *testing.T) {
	executed := false
	expectedErr := errors.New("function error")
	err := base.WithLock(t.Context(), nil, "provider", "encodedKey", func(ctx context.Context) error {
		executed = true
		return expectedErr
	})
	require.True(t, executed, "expected function to be executed")
	require.ErrorIs(t, err, expectedErr)
}

func TestWithLock_WithLocker(t *testing.T) {
	mock := &mockLocker{}
	err := base.WithLock(t.Context(), mock, "provider", "encodedKey", func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "provider:encodedKey", mock.recordedKey)
}

func TestWithLockResult_NilLocker(t *testing.T) {
	result, err := base.WithLockResult(t.Context(), nil, "provider", "key", func(ctx context.Context) (string, error) {
		return "test result", nil
	})
	require.NoError(t, err)
	require.Equal(t, "test result", result)
}

func TestWithLockResult_WithLocker(t *testing.T) {
	mock := &mockLocker{}
	result, err := base.WithLockResult(t.Context(), mock, "provider", "key", func(ctx context.Context) (int, error) {
		return 42, nil
	})
	require.NoError(t, err)
	require.Equal(t, 42, result)
}

func TestWithLockResult_Error(t *testing.T) {
	expectedErr := errors.New("function error")
	result, err := base.WithLockResult(t.Context(), &mockLocker{}, "p", "k", func(ctx context.Context) (string, error) {
		return "", expectedErr
	})
	require.ErrorIs(t, err, expectedErr)
	require.Empty(t, result)
}

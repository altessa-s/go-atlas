// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base_test

import (
	"context"
	"errors"
	"testing"

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
	if !executed {
		t.Error("expected function to be executed")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestWithLock_WithLocker(t *testing.T) {
	mock := &mockLocker{}
	err := base.WithLock(t.Context(), mock, "provider", "encodedKey", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mock.recordedKey != "provider:encodedKey" {
		t.Errorf("lock key = %q, want %q", mock.recordedKey, "provider:encodedKey")
	}
}

func TestWithLockResult_NilLocker(t *testing.T) {
	result, err := base.WithLockResult(t.Context(), nil, "provider", "key", func(ctx context.Context) (string, error) {
		return "test result", nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "test result" {
		t.Errorf("result = %q, want %q", result, "test result")
	}
}

func TestWithLockResult_WithLocker(t *testing.T) {
	mock := &mockLocker{}
	result, err := base.WithLockResult(t.Context(), mock, "provider", "key", func(ctx context.Context) (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("result = %d, want 42", result)
	}
}

func TestWithLockResult_Error(t *testing.T) {
	expectedErr := errors.New("function error")
	result, err := base.WithLockResult(t.Context(), &mockLocker{}, "p", "k", func(ctx context.Context) (string, error) {
		return "", expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if result != "" {
		t.Errorf("expected zero value, got %q", result)
	}
}

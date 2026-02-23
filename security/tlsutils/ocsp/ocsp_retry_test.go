// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"errors"
	"testing"
	"time"
)

type mockRetryPolicy struct {
	maxAttempts int
	nextDelay   time.Duration
	shouldRetry bool
}

func (m *mockRetryPolicy) NextRetry(_ int) time.Duration { return m.nextDelay }
func (m *mockRetryPolicy) MaxAttempts() int              { return m.maxAttempts }
func (m *mockRetryPolicy) ShouldRetry(_ error) bool      { return m.shouldRetry }

func TestExecuteWithRetry_NilConfig(t *testing.T) {
	called := false
	err := ExecuteWithRetry(func() error {
		called = true
		return nil
	}, nil)
	if err != nil {
		t.Errorf("ExecuteWithRetry(nil config) = %v, want nil", err)
	}
	if !called {
		t.Error("function was not called")
	}
}

func TestExecuteWithRetry_NilPolicy(t *testing.T) {
	called := false
	err := ExecuteWithRetry(func() error {
		called = true
		return nil
	}, &RetryConfig{Policy: nil})
	if err != nil {
		t.Errorf("ExecuteWithRetry(nil policy) = %v, want nil", err)
	}
	if !called {
		t.Error("function was not called")
	}
}

func TestExecuteWithRetry_Success(t *testing.T) {
	policy := &mockRetryPolicy{maxAttempts: 3, nextDelay: time.Millisecond, shouldRetry: true}
	err := ExecuteWithRetry(func() error {
		return nil
	}, &RetryConfig{Policy: policy, Context: t.Context()})
	if err != nil {
		t.Errorf("ExecuteWithRetry(success) = %v, want nil", err)
	}
}

func TestExecuteWithRetry_PermanentError(t *testing.T) {
	testErr := errors.New("permanent error")
	policy := &mockRetryPolicy{maxAttempts: 3, nextDelay: time.Millisecond, shouldRetry: false}
	err := ExecuteWithRetry(func() error {
		return testErr
	}, &RetryConfig{Policy: policy, Context: t.Context()})
	if err == nil {
		t.Error("ExecuteWithRetry(permanent error) should return error")
	}
}

func TestExecuteWithRetry_WithOnRetry(t *testing.T) {
	attempts := 0
	policy := &mockRetryPolicy{maxAttempts: 2, nextDelay: time.Millisecond, shouldRetry: true}

	retryCount := 0
	_ = ExecuteWithRetry(func() error {
		attempts++
		return errors.New("transient")
	}, &RetryConfig{
		Policy:  policy,
		Context: t.Context(),
		OnRetry: func(attempt int, err error, nextDelay time.Duration) {
			retryCount++
		},
	})

	if attempts == 0 {
		t.Error("function was never called")
	}
}

func TestRetryConfig_NilContext(t *testing.T) {
	policy := &mockRetryPolicy{maxAttempts: 1, nextDelay: 0, shouldRetry: false}
	err := ExecuteWithRetry(func() error {
		return nil
	}, &RetryConfig{Policy: policy, Context: nil})
	if err != nil {
		t.Errorf("ExecuteWithRetry(nil ctx) = %v, want nil", err)
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease_test

import (
	"context"
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := natskvlease.DefaultRetryConfig()
	if cfg.MaxRetries != natskvlease.DefaultMaxRetries {
		t.Errorf("MaxRetries = %d, want %d", cfg.MaxRetries, natskvlease.DefaultMaxRetries)
	}
	if cfg.MaxElapsedTime != natskvlease.DefaultMaxElapsedTime {
		t.Errorf("MaxElapsedTime = %v, want %v", cfg.MaxElapsedTime, natskvlease.DefaultMaxElapsedTime)
	}
}

func TestRetry_Success(t *testing.T) {
	ctx := t.Context()
	result, err := natskvlease.Retry(ctx, func() (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("Retry() error: %v", err)
	}
	if result != "ok" {
		t.Errorf("Retry() = %q, want %q", result, "ok")
	}
}

func TestRetry_PermanentError(t *testing.T) {
	ctx := t.Context()
	permErr := errors.New("permanent error")

	_, err := natskvlease.Retry(ctx, func() (string, error) {
		return "", permErr
	})
	if err == nil {
		t.Fatal("Retry() should return error for permanent errors")
	}
}

func TestRetryWithConfig_Success(t *testing.T) {
	ctx := t.Context()
	cfg := natskvlease.RetryConfig{MaxRetries: 3, MaxElapsedTime: natskvlease.DefaultMaxElapsedTime}

	result, err := natskvlease.RetryWithConfig(ctx, func() (int, error) {
		return 42, nil
	}, cfg)
	if err != nil {
		t.Fatalf("RetryWithConfig() error: %v", err)
	}
	if result != 42 {
		t.Errorf("RetryWithConfig() = %d, want 42", result)
	}
}

func TestRetryWithConfig_ZeroRetries(t *testing.T) {
	ctx := t.Context()
	cfg := natskvlease.RetryConfig{MaxRetries: 0, MaxElapsedTime: natskvlease.DefaultMaxElapsedTime}

	_, err := natskvlease.RetryWithConfig(ctx, func() (string, error) {
		return "", errors.New("fail")
	}, cfg)
	if err == nil {
		t.Fatal("RetryWithConfig() with MaxRetries=0 should return error")
	}
}

func TestRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := natskvlease.Retry(ctx, func() (string, error) {
		return "", errors.New("should not retry")
	})
	if err == nil {
		t.Fatal("Retry() should return error when context is canceled")
	}
}

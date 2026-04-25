// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
)

func TestRetry_Success(t *testing.T) {
	ctx := t.Context()
	result, err := natskvlease.Retry(ctx, func() (string, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	require.Equal(t, "ok", result)
}

func TestRetry_PermanentError(t *testing.T) {
	ctx := t.Context()
	permErr := errors.New("permanent error")

	_, err := natskvlease.Retry(ctx, func() (string, error) {
		return "", permErr
	})
	require.Error(t, err)
}

func TestRetryWithConfig_Success(t *testing.T) {
	ctx := t.Context()
	cfg := natskvlease.RetryConfig{MaxRetries: 3, MaxElapsedTime: natskvlease.DefaultMaxElapsedTime}

	result, err := natskvlease.RetryWithConfig(ctx, func() (int, error) {
		return 42, nil
	}, cfg)
	require.NoError(t, err)
	require.Equal(t, 42, result)
}

func TestRetryWithConfig_ZeroRetries(t *testing.T) {
	ctx := t.Context()
	cfg := natskvlease.RetryConfig{MaxRetries: 0, MaxElapsedTime: natskvlease.DefaultMaxElapsedTime}

	_, err := natskvlease.RetryWithConfig(ctx, func() (string, error) {
		return "", errors.New("fail")
	}, cfg)
	require.Error(t, err)
}

func TestRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := natskvlease.Retry(ctx, func() (string, error) {
		return "", errors.New("should not retry")
	})
	require.Error(t, err)
}

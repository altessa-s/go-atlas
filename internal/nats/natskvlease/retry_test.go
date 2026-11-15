// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
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

func TestRetry_WithOptions_Success(t *testing.T) {
	ctx := t.Context()

	result, err := natskvlease.Retry(ctx, func() (int, error) {
		return 42, nil
	}, natskvlease.WithRetryMaxAttempts(3), natskvlease.WithRetryJitter(0))
	require.NoError(t, err)
	require.Equal(t, 42, result)
}

// A transient failure is retried until the attempt budget runs out. Zero
// attempts is not "unlimited" and not "the default": it behaves like one.
func TestRetry_MaxAttempts(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		opts []natskvlease.RetryOption
		want int
	}{
		"zero": {opts: []natskvlease.RetryOption{natskvlease.WithRetryMaxAttempts(0)}, want: 1},
		"one":  {opts: []natskvlease.RetryOption{natskvlease.WithRetryMaxAttempts(1)}, want: 1},
		"two":  {opts: []natskvlease.RetryOption{natskvlease.WithRetryMaxAttempts(2)}, want: 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			transient := &testhelpers.MockNetError{IsTimeout: true, Msg: "timeout"}
			// A generous time budget, so only the attempt bound ends the retry.
			opts := append([]natskvlease.RetryOption{natskvlease.WithRetryMaxElapsedTime(time.Minute), natskvlease.WithRetryJitter(0)}, tc.opts...)
			_, err := natskvlease.Retry(t.Context(), func() (string, error) {
				calls++
				return "", transient
			}, opts...)
			require.ErrorIs(t, err, transient)
			require.Equal(t, tc.want, calls)
		})
	}
}

func TestRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := natskvlease.Retry(ctx, func() (string, error) {
		return "", errors.New("should not retry")
	})
	require.Error(t, err)
}

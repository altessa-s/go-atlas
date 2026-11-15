// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreretry "github.com/altessa-s/go-atlas-core/retry"
)

var errProbe = errors.New("no reachable primary")

func fastBackoff() coreretry.NextDelayFunc {
	return coreretry.Exponential(time.Millisecond, coreretry.WithExponentialMaxDelay(5*time.Millisecond))
}

// A probe that fails says nothing about the deployment, so it is repeated
// until the deployment answers — whichever way.
func TestProbeUntilAnswered_RetriesFailedProbes(t *testing.T) {
	t.Parallel()

	for _, supported := range []bool{true, false} {
		calls := 0
		got, err := probeUntilAnswered(t.Context(), fastBackoff(), func(context.Context) (bool, error) {
			calls++
			if calls <= 3 {
				return false, errProbe
			}
			return supported, nil
		})
		require.NoError(t, err)
		require.Equal(t, supported, got)
		require.Equal(t, 4, calls)
	}
}

// An answer — including "unsupported" — is final and costs one probe.
func TestProbeUntilAnswered_AnswerIsNotRetried(t *testing.T) {
	t.Parallel()

	calls := 0
	got, err := probeUntilAnswered(t.Context(), fastBackoff(), func(context.Context) (bool, error) {
		calls++
		return false, nil
	})
	require.NoError(t, err)
	require.False(t, got)
	require.Equal(t, 1, calls)
}

// Each attempt gets its own deadline, so a hung handshake is abandoned and
// retried instead of blocking the watcher for good.
func TestProbeUntilAnswered_BoundsEachAttempt(t *testing.T) {
	t.Parallel()

	_, err := probeUntilAnswered(t.Context(), fastBackoff(), func(ctx context.Context) (bool, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), watchProbeTimeout)
		return true, nil
	})
	require.NoError(t, err)
}

// Only the watch context ends the wait for an answer.
func TestProbeUntilAnswered_StopsWhenContextEnds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	_, err := probeUntilAnswered(ctx, fastBackoff(), func(context.Context) (bool, error) {
		calls++
		if calls == 3 {
			cancel()
		}
		return false, errProbe
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 3, calls)
}

// Shutdown before the deployment ever answered is a clean stop, not an error
// and not a missing capability.
func TestWatch_ShutdownBeforeTheProbeAnswers(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.NoError(t, (&Store{}).Watch(ctx, func() {}))
}

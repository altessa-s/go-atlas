// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/client/limiters"
)

// recording returns a limiter that appends name to calls and returns err.
func recording(calls *[]string, name string, err error) limiters.RequestsLimiter {
	return limiters.Func(func(_ context.Context, key string) error {
		*calls = append(*calls, name+":"+key)
		return err
	})
}

func TestFunc(t *testing.T) {
	t.Parallel()

	errDenied := errors.New("denied")
	var got string
	l := limiters.Func(func(_ context.Context, key string) error {
		got = key
		return errDenied
	})
	require.ErrorIs(t, l.Allow(t.Context(), "api.example.com"), errDenied)
	require.Equal(t, "api.example.com", got)
}

func TestChain(t *testing.T) {
	t.Parallel()

	errBudget := errors.New("budget exhausted")

	t.Run("consults in order", func(t *testing.T) {
		t.Parallel()

		var calls []string
		c := limiters.Chain(recording(&calls, "budget", nil), recording(&calls, "pacer", nil))
		require.NoError(t, c.Allow(t.Context(), "h"))
		require.Equal(t, []string{"budget:h", "pacer:h"}, calls)
	})

	t.Run("stops at the first refusal", func(t *testing.T) {
		t.Parallel()

		var calls []string
		c := limiters.Chain(recording(&calls, "budget", errBudget), recording(&calls, "pacer", nil))
		require.ErrorIs(t, c.Allow(t.Context(), "h"), errBudget)
		require.Equal(t, []string{"budget:h"}, calls, "a refused request takes nothing from the pacer")
	})

	t.Run("skips nil and collapses", func(t *testing.T) {
		t.Parallel()

		var typedNil limiters.Func
		require.Nil(t, limiters.Chain())
		require.Nil(t, limiters.Chain(nil, typedNil))

		var calls []string
		only := recording(&calls, "pacer", nil)
		single := limiters.Chain(nil, only)
		require.NoError(t, single.Allow(t.Context(), "h"))
		require.Equal(t, []string{"pacer:h"}, calls, "a single limiter is returned as is")
	})
}

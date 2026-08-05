// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// resetShutdownStateForTest clears the package-level registry so
// multiple subtests can exercise RunShutdownHooks independently. It
// deliberately lives in the test file; production code must not call it.
func resetShutdownStateForTest(t *testing.T) {
	t.Helper()
	processHooks.mu.Lock()
	processHooks.hooks = nil
	processHooks.once = sync.Once{}
	processHooks.mu.Unlock()
}

func TestOnShutdownRegistersHook(t *testing.T) {
	resetShutdownStateForTest(t)

	var calls []int
	OnShutdown(func(_ context.Context) error {
		calls = append(calls, 1)
		return nil
	})
	OnShutdown(func(_ context.Context) error {
		calls = append(calls, 2)
		return nil
	})

	require.NoError(t, RunShutdownHooks(t.Context()))

	// LIFO: last registered runs first.
	require.Equal(t, []int{2, 1}, calls, "expected LIFO order")
}

func TestRunShutdownHooksJoinsErrors(t *testing.T) {
	resetShutdownStateForTest(t)

	errA := errors.New("hook a failed")
	errB := errors.New("hook b failed")

	var order []string
	OnShutdown(func(_ context.Context) error {
		order = append(order, "a")
		return errA
	})
	OnShutdown(func(_ context.Context) error {
		order = append(order, "b")
		return errB
	})
	OnShutdown(func(_ context.Context) error {
		order = append(order, "c")
		return nil
	})

	err := RunShutdownHooks(t.Context())
	require.Error(t, err)
	// Both error values should be reachable via errors.Is since
	// RunShutdownHooks uses errors.Join on wrapped errors.
	require.ErrorIs(t, err, errA)
	require.ErrorIs(t, err, errB)

	// LIFO execution continues despite errors.
	require.Equal(t, []string{"c", "b", "a"}, order)
}

func TestRunShutdownHooksRunsAtMostOnce(t *testing.T) {
	resetShutdownStateForTest(t)

	var count int
	OnShutdown(func(_ context.Context) error {
		count++
		return nil
	})

	require.NoError(t, RunShutdownHooks(t.Context()), "first call")
	require.NoError(t, RunShutdownHooks(t.Context()), "second call")
	require.Equal(t, 1, count, "hook should run exactly once")
}

func TestRunShutdownHooksNoRegistrations(t *testing.T) {
	resetShutdownStateForTest(t)

	require.NoError(t, RunShutdownHooks(t.Context()))
}

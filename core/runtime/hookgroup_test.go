// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package runtime_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
)

func TestHookGroup_ZeroValueIsUsable(t *testing.T) {
	t.Parallel()

	var group coreruntime.HookGroup
	require.NoError(t, group.Shutdown(t.Context()))
}

func TestHookGroup_RunsLIFO(t *testing.T) {
	t.Parallel()

	var (
		group coreruntime.HookGroup
		order []int
	)

	for i := range 3 {
		group.OnShutdown(func(_ context.Context) error {
			order = append(order, i)
			return nil
		})
	}

	require.NoError(t, group.Shutdown(t.Context()))
	require.Equal(t, []int{2, 1, 0}, order, "resources registered first must be cleaned up last")
}

func TestHookGroup_JoinsErrorsAndKeepsGoing(t *testing.T) {
	t.Parallel()

	errFirst := errors.New("first failed")
	errSecond := errors.New("second failed")

	var (
		group coreruntime.HookGroup
		ran   int
	)

	group.OnShutdown(func(_ context.Context) error { ran++; return errFirst })
	group.OnShutdown(func(_ context.Context) error { ran++; return errSecond })
	group.OnShutdown(func(_ context.Context) error { ran++; return nil })

	err := group.Shutdown(t.Context())
	require.ErrorIs(t, err, errFirst)
	require.ErrorIs(t, err, errSecond)
	require.Equal(t, 3, ran, "a failing hook must not strand the resources the others release")
}

func TestHookGroup_RunsAtMostOnce(t *testing.T) {
	t.Parallel()

	var (
		group coreruntime.HookGroup
		count int
	)

	group.OnShutdown(func(_ context.Context) error { count++; return nil })

	require.NoError(t, group.Shutdown(t.Context()))
	require.NoError(t, group.Shutdown(t.Context()))
	require.Equal(t, 1, count)
}

// A hook registered while Shutdown is running must not extend the sequence it
// is part of, otherwise a self-registering hook loops forever.
func TestHookGroup_IgnoresHooksRegisteredDuringShutdown(t *testing.T) {
	t.Parallel()

	var (
		group coreruntime.HookGroup
		late  int
	)

	group.OnShutdown(func(_ context.Context) error {
		group.OnShutdown(func(_ context.Context) error { late++; return nil })
		return nil
	})

	require.NoError(t, group.Shutdown(t.Context()))
	require.Zero(t, late)
}

// The point of a group: two scopes shut down independently, unlike the
// process-wide registry, which runs once for the whole program.
func TestHookGroup_ScopesAreIndependent(t *testing.T) {
	t.Parallel()

	var (
		first, second coreruntime.HookGroup
		firstRan      bool
		secondRan     bool
	)

	first.OnShutdown(func(_ context.Context) error { firstRan = true; return nil })
	second.OnShutdown(func(_ context.Context) error { secondRan = true; return nil })

	require.NoError(t, first.Shutdown(t.Context()))
	require.True(t, firstRan)
	require.False(t, secondRan, "shutting down one scope must not stop the other")

	require.NoError(t, second.Shutdown(t.Context()))
	require.True(t, secondRan)
}

func TestHookGroup_ConcurrentRegistration(t *testing.T) {
	t.Parallel()

	var (
		group coreruntime.HookGroup
		mu    sync.Mutex
		count int
	)

	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			group.OnShutdown(func(_ context.Context) error {
				mu.Lock()
				defer mu.Unlock()
				count++

				return nil
			})
		})
	}
	wg.Wait()

	require.NoError(t, group.Shutdown(t.Context()))
	require.Equal(t, 32, count)
}

func TestHookGroup_ConcurrentShutdown(t *testing.T) {
	t.Parallel()

	var (
		group coreruntime.HookGroup
		mu    sync.Mutex
		count int
	)

	group.OnShutdown(func(_ context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		count++

		return nil
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_ = group.Shutdown(t.Context())
		})
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, count)
}

func TestHookGroup_PassesContextToHooks(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}

	var (
		group coreruntime.HookGroup
		got   any
	)

	group.OnShutdown(func(ctx context.Context) error {
		got = ctx.Value(ctxKey{})
		return nil
	})

	//nolint:staticcheck // SA1029: a struct key is fine; this ctx never leaves the test.
	require.NoError(t, group.Shutdown(context.WithValue(t.Context(), ctxKey{}, "deadline-carrier")))
	require.Equal(t, "deadline-carrier", got)
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// resetShutdownStateForTest clears the package-level registry so
// multiple subtests can exercise RunShutdownHooks independently. It
// deliberately lives in the test file; production code must not call it.
func resetShutdownStateForTest(t *testing.T) {
	t.Helper()
	shutdownMu.Lock()
	shutdownHooks = nil
	shutdownOnce = sync.Once{}
	shutdownMu.Unlock()
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

	if err := RunShutdownHooks(context.Background()); err != nil {
		t.Fatalf("RunShutdownHooks: %v", err)
	}

	// LIFO: last registered runs first.
	if len(calls) != 2 || calls[0] != 2 || calls[1] != 1 {
		t.Fatalf("expected LIFO order [2,1], got %v", calls)
	}
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

	err := RunShutdownHooks(context.Background())
	if err == nil {
		t.Fatal("expected joined error, got nil")
	}
	// Both error values should be reachable via errors.Is since
	// RunShutdownHooks uses errors.Join on wrapped errors.
	if !errors.Is(err, errA) {
		t.Errorf("errors.Is(err, errA) = false, want true")
	}
	if !errors.Is(err, errB) {
		t.Errorf("errors.Is(err, errB) = false, want true")
	}

	// LIFO execution continues despite errors.
	wantOrder := []string{"c", "b", "a"}
	if len(order) != len(wantOrder) {
		t.Fatalf("order=%v, want %v", order, wantOrder)
	}
	for i, v := range wantOrder {
		if order[i] != v {
			t.Fatalf("order=%v, want %v", order, wantOrder)
		}
	}
}

func TestRunShutdownHooksRunsAtMostOnce(t *testing.T) {
	resetShutdownStateForTest(t)

	var count int
	OnShutdown(func(_ context.Context) error {
		count++
		return nil
	})

	ctx := context.Background()
	if err := RunShutdownHooks(ctx); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := RunShutdownHooks(ctx); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if count != 1 {
		t.Fatalf("hook ran %d times, want 1", count)
	}
}

func TestRunShutdownHooksNoRegistrations(t *testing.T) {
	resetShutdownStateForTest(t)

	if err := RunShutdownHooks(context.Background()); err != nil {
		t.Fatalf("RunShutdownHooks with no hooks: %v", err)
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package panics_test

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/panics"

	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
)

// TestHandle_SwallowedPanic_DoesNotConsumeShutdownHooks guards against the
// regression where every recovered panic ran the process-wide shutdown hooks,
// consuming their once-only budget so a later real shutdown silently skipped
// them. Hooks may only run on the re-panic path.
func TestHandle_SwallowedPanic_DoesNotConsumeShutdownHooks(t *testing.T) {
	var hookRan atomic.Bool
	coreruntime.OnShutdown(func(context.Context) error {
		hookRan.Store(true)
		return nil
	})

	func() {
		defer panics.HandleWithOpts(t.Context(), panics.NewHandleOpts().SetReallyPanic(false))
		panic("routine panic")
	}()

	require.False(t, hookRan.Load(),
		"a recovered-and-swallowed panic must not run process-wide shutdown hooks")
}

func TestNewHandleOpts(t *testing.T) {
	opts := panics.NewHandleOpts()
	require.NotNil(t, opts, "NewHandleOpts() returned nil")
	require.Nil(t, opts.ReallyPanic, "ReallyPanic should be nil by default")
}

func TestHandleOpts_SetReallyPanic(t *testing.T) {
	opts := panics.NewHandleOpts().SetReallyPanic(true)
	require.NotNil(t, opts.ReallyPanic)
	require.True(t, *opts.ReallyPanic, "SetReallyPanic(true) should set ReallyPanic to true")

	opts2 := panics.NewHandleOpts().SetReallyPanic(false)
	require.NotNil(t, opts2.ReallyPanic)
	require.False(t, *opts2.ReallyPanic, "SetReallyPanic(false) should set ReallyPanic to false")
}

func TestHandleWithOpts_ReallyPanicTrue(t *testing.T) {
	defer func() {
		require.NotNil(t, recover(), "expected panic to propagate with ReallyPanic=true")
	}()

	opts := panics.NewHandleOpts().SetReallyPanic(true)
	func() {
		defer panics.HandleWithOpts(t.Context(), opts)
		panic("test panic")
	}()
}

func TestHandleWithOpts_ReallyPanicFalse(t *testing.T) {
	opts := panics.NewHandleOpts().SetReallyPanic(false)

	var recovered bool
	handler := func(_ context.Context, r any) {
		recovered = true
	}

	func() {
		defer panics.HandleWithOpts(t.Context(), opts, handler)
		panic("test panic")
	}()

	require.True(t, recovered, "handler should have been called")
}

func TestHandle_WithCustomHandler(t *testing.T) {
	var got any
	handler := func(_ context.Context, r any) {
		got = r
	}

	panics.SetReallyPanic(false)
	defer panics.SetReallyPanic(false)

	func() {
		defer panics.Handle(t.Context(), handler)
		panic("custom panic value")
	}()

	require.Equal(t, "custom panic value", got)
}

func TestMustNonNil_NonNilDoesNotPanic(t *testing.T) {
	msg := "ptr msg"
	// Non-nil value should not panic
	panics.MustNonNil(&msg, &msg)
}

func TestMustNonNil_NilWithPointerMessage(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
	}()

	msg := "must not be nil"
	panics.MustNonNil(nil, &msg)
}

func TestMust_StringPointerMessage(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic")
		s, ok := r.(string)
		require.True(t, ok, "expected string, got %T", r)
		require.Equal(t, "condition failed", s)
	}()

	msg := "condition failed"
	panics.Must(false, &msg)
}

func TestSetLoggerFromContext_CustomFunc(t *testing.T) {
	// Just ensure no panic when setting
	panics.SetLoggerFromContext(func(ctx context.Context) *slog.Logger {
		return slog.Default()
	})
}

func TestSetLogger_Default(t *testing.T) {
	// Just ensure no panic
	panics.SetLogger(slog.Default())
}

func TestAddGlobalPanicHandler(t *testing.T) {
	var handlerCalled bool
	panics.AddGlobalPanicHandler(func(_ context.Context, _ any) {
		handlerCalled = true
	})

	panics.SetReallyPanic(false)
	defer panics.SetReallyPanic(false)

	func() {
		defer panics.Handle(t.Context())
		panic("test")
	}()

	require.True(t, handlerCalled, "global handler should have been called")

	// Restore defaults
	panics.SetGlobalPanicHandlers()
}

// TestAddGlobalPanicHandler_ConcurrentRegistration guards against the
// lost-update race: AddGlobalPanicHandler used to do a load-modify-store
// on the atomic.Value, so two concurrent registrations could both build
// on the same base slice and one handler silently vanished.
func TestAddGlobalPanicHandler_ConcurrentRegistration(t *testing.T) {
	panics.SetGlobalPanicHandlers() // start from a clean slate
	t.Cleanup(func() { panics.SetGlobalPanicHandlers() })

	const n = 64
	var calls atomic.Int32
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			panics.AddGlobalPanicHandler(func(context.Context, any) {
				calls.Add(1)
			})
		})
	}
	wg.Wait()

	func() {
		defer panics.HandleWithOpts(t.Context(), panics.NewHandleOpts().SetReallyPanic(false))
		panic("boom")
	}()

	require.Equal(t, int32(n), calls.Load(), "every concurrently registered handler must run")
}

func TestInvalidArgument_Triggers(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic from InvalidArgument")
		}
	}()

	panics.InvalidArgument(true, "bad arg")
}

func TestInvalidArgument_NoTrigger(t *testing.T) {
	// Should not panic
	panics.InvalidArgument(false, "ok")
}

func TestMustResult_Success(t *testing.T) {
	require.Equal(t, 42, panics.MustResult(42, nil))
}

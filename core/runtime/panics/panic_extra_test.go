// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package panics_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
)

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

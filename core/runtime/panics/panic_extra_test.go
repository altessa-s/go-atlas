// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package panics_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
)

func TestNewHandleOpts(t *testing.T) {
	opts := panics.NewHandleOpts()
	if opts == nil {
		t.Fatal("NewHandleOpts() returned nil")
	}
	if opts.ReallyPanic != nil {
		t.Error("ReallyPanic should be nil by default")
	}
}

func TestHandleOpts_SetReallyPanic(t *testing.T) {
	opts := panics.NewHandleOpts().SetReallyPanic(true)
	if opts.ReallyPanic == nil || !*opts.ReallyPanic {
		t.Error("SetReallyPanic(true) should set ReallyPanic to true")
	}

	opts2 := panics.NewHandleOpts().SetReallyPanic(false)
	if opts2.ReallyPanic == nil || *opts2.ReallyPanic {
		t.Error("SetReallyPanic(false) should set ReallyPanic to false")
	}
}

func TestHandleWithOpts_ReallyPanicTrue(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic to propagate with ReallyPanic=true")
		}
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

	if !recovered {
		t.Error("handler should have been called")
	}
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

	if got != "custom panic value" {
		t.Errorf("handler got %v, want 'custom panic value'", got)
	}
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
		if r == nil {
			t.Fatal("expected panic")
		}
		if s, ok := r.(string); !ok || s != "condition failed" {
			t.Errorf("got %v, want 'condition failed'", r)
		}
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

	if !handlerCalled {
		t.Error("global handler should have been called")
	}

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
	val := panics.MustResult(42, nil)
	if val != 42 {
		t.Errorf("MustResult() = %d, want 42", val)
	}
}

func TestErrInvalidArgument(t *testing.T) {
	if panics.ErrInvalidArgument == nil {
		t.Error("ErrInvalidArgument should not be nil")
	}
}

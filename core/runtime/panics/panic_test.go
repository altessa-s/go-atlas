// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package panics

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetGlobalPanicHandlers_StoresCopy(t *testing.T) {
	h := func(context.Context, any) {}

	SetGlobalPanicHandlers(h)
	gotAny := globalPanicHandlers.Load()
	got, ok := gotAny.(PanicHandlers)
	require.True(t, ok, "unexpected type in globalPanicHandlers: %T", gotAny)
	require.Len(t, got, 1)

	// Mutate the original slice passed to SetGlobalPanicHandlers (best-effort).
	handlers := []PanicHandler{h}
	SetGlobalPanicHandlers(handlers...)
	handlers[0] = nil

	gotAny2 := globalPanicHandlers.Load()
	got2 := gotAny2.(PanicHandlers)
	require.NotNil(t, got2[0], "stored handlers were mutated via external slice; expected copy")
}

func TestAddGlobalPanicHandler_NilIsIgnored(t *testing.T) {
	// Ensure no panic and no change to state beyond what's already there.
	before := globalPanicHandlers.Load().(PanicHandlers)
	AddGlobalPanicHandler(nil)
	after := globalPanicHandlers.Load().(PanicHandlers)
	require.Len(t, after, len(before))
}

func TestMustNonNil(t *testing.T) {
	// Non-nil should not panic
	MustNonNil("value", "should not panic")

	// Nil should panic
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil")
		}
	}()
	MustNonNil(nil, "nil value")
}

func TestMustNonZero(t *testing.T) {
	MustNonZero(42, "should not panic")

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for zero")
		}
	}()
	MustNonZero(0, "zero value")
}

func TestMustError(t *testing.T) {
	// nil error should not panic
	MustError(nil)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-nil error")
		}
	}()
	MustError(fmt.Errorf("bad"))
}

func TestMustResult(t *testing.T) {
	got := MustResult(42, nil)
	require.Equal(t, 42, got)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	MustResult(0, fmt.Errorf("error"))
}

func TestMust(t *testing.T) {
	Must(true, "should not panic")

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for false")
		}
	}()
	Must(false, "condition failed")
}

func TestInvalidArgument(t *testing.T) {
	// ok=false should NOT panic
	InvalidArgument(false, "fine")

	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic")
		err, ok := r.(error)
		require.True(t, ok, "expected error, got %T", r)
		require.ErrorIs(t, err, ErrInvalidArgument)
	}()
	InvalidArgument(true, "bad arg")
}

func TestHandle(t *testing.T) {
	var captured any
	func() {
		defer Handle(t.Context(), func(_ context.Context, r any) {
			captured = r
		})
		panic("test panic")
	}()
	require.Equal(t, "test panic", captured)
}

func TestHandleWithOpts(t *testing.T) {
	var captured any
	func() {
		opts := NewHandleOpts().SetReallyPanic(false)
		defer HandleWithOpts(t.Context(), opts, func(_ context.Context, r any) {
			captured = r
		})
		panic("opts panic")
	}()
	require.Equal(t, "opts panic", captured)
}

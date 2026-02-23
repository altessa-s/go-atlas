// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package panics

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestSetGlobalPanicHandlers_StoresCopy(t *testing.T) {
	h := func(context.Context, any) {}

	SetGlobalPanicHandlers(h)
	gotAny := globalPanicHandlers.Load()
	got, ok := gotAny.(PanicHandlers)
	if !ok {
		t.Fatalf("unexpected type in globalPanicHandlers: %T", gotAny)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}

	// Mutate the original slice passed to SetGlobalPanicHandlers (best-effort).
	handlers := []PanicHandler{h}
	SetGlobalPanicHandlers(handlers...)
	handlers[0] = nil

	gotAny2 := globalPanicHandlers.Load()
	got2 := gotAny2.(PanicHandlers)
	if got2[0] == nil {
		t.Fatalf("stored handlers were mutated via external slice; expected copy")
	}
}

func TestAddGlobalPanicHandler_NilIsIgnored(t *testing.T) {
	// Ensure no panic and no change to state beyond what's already there.
	before := globalPanicHandlers.Load().(PanicHandlers)
	AddGlobalPanicHandler(nil)
	after := globalPanicHandlers.Load().(PanicHandlers)
	if len(after) != len(before) {
		t.Fatalf("len(after)=%d, want %d", len(after), len(before))
	}
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
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}

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
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("expected error, got %T", r)
		}
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("expected ErrInvalidArgument, got %v", err)
		}
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
	if captured != "test panic" {
		t.Fatalf("expected 'test panic', got %v", captured)
	}
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
	if captured != "opts panic" {
		t.Fatalf("expected 'opts panic', got %v", captured)
	}
}

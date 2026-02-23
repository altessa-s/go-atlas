// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"fmt"
	"io/fs"
	"os"
	"testing"

	"github.com/altessa-s/go-atlas/core/errors"

	std_errors "errors"
)

type testAppError struct {
	Code    int
	Message string
}

func (e *testAppError) Error() string {
	return fmt.Sprintf("app error %d: %s", e.Code, e.Message)
}

// customAsError delegates matching via As(any) bool method.
type customAsError struct {
	code int
}

func (e *customAsError) Error() string { return fmt.Sprintf("custom %d", e.code) }

func (e *customAsError) As(target any) bool {
	if t, ok := target.(**testAppError); ok {
		*t = &testAppError{Code: e.code, Message: "from As"}
		return true
	}
	return false
}

func TestAsType(t *testing.T) {
	t.Run("matches pointer type", func(t *testing.T) {
		original := &testAppError{Code: 404, Message: "not found"}
		err := fmt.Errorf("wrapped: %w", original)

		target, ok := errors.AsType[*testAppError](err)
		if !ok {
			t.Fatal("expected match")
		}
		if target.Code != 404 {
			t.Errorf("Code = %d, want 404", target.Code)
		}
		if target.Message != "not found" {
			t.Errorf("Message = %q, want %q", target.Message, "not found")
		}
	})

	t.Run("matches interface type", func(t *testing.T) {
		err := fmt.Errorf("wrapped: %w", &fs.PathError{Op: "open", Path: "/tmp/x", Err: os.ErrNotExist})

		target, ok := errors.AsType[*fs.PathError](err)
		if !ok {
			t.Fatal("expected match")
		}
		if target.Op != "open" {
			t.Errorf("Op = %q, want %q", target.Op, "open")
		}
	})

	t.Run("no match returns zero value and false", func(t *testing.T) {
		err := std_errors.New("plain error")

		target, ok := errors.AsType[*testAppError](err)
		if ok {
			t.Fatal("expected no match")
		}
		if target != nil {
			t.Errorf("target = %v, want nil", target)
		}
	})

	t.Run("nil error returns zero value and false", func(t *testing.T) {
		target, ok := errors.AsType[*testAppError](nil)
		if ok {
			t.Fatal("expected no match for nil")
		}
		if target != nil {
			t.Errorf("target = %v, want nil", target)
		}
	})

	t.Run("deeply wrapped error", func(t *testing.T) {
		original := &testAppError{Code: 500, Message: "internal"}
		err := fmt.Errorf("level3: %w", fmt.Errorf("level2: %w", fmt.Errorf("level1: %w", original)))

		target, ok := errors.AsType[*testAppError](err)
		if !ok {
			t.Fatal("expected match through wrapping chain")
		}
		if target.Code != 500 {
			t.Errorf("Code = %d, want 500", target.Code)
		}
	})

	t.Run("custom As method", func(t *testing.T) {
		err := &customAsError{code: 42}

		target, ok := errors.AsType[*testAppError](err)
		if !ok {
			t.Fatal("expected match via As method")
		}
		if target.Code != 42 {
			t.Errorf("Code = %d, want 42", target.Code)
		}
		if target.Message != "from As" {
			t.Errorf("Message = %q, want %q", target.Message, "from As")
		}
	})

	t.Run("custom As method wrapped", func(t *testing.T) {
		err := fmt.Errorf("outer: %w", &customAsError{code: 99})

		target, ok := errors.AsType[*testAppError](err)
		if !ok {
			t.Fatal("expected match via As method in chain")
		}
		if target.Code != 99 {
			t.Errorf("Code = %d, want 99", target.Code)
		}
	})

	t.Run("errors.Join multi-error", func(t *testing.T) {
		err := std_errors.Join(
			std_errors.New("first"),
			&testAppError{Code: 503, Message: "unavailable"},
			std_errors.New("third"),
		)

		target, ok := errors.AsType[*testAppError](err)
		if !ok {
			t.Fatal("expected match in joined errors")
		}
		if target.Code != 503 {
			t.Errorf("Code = %d, want 503", target.Code)
		}
	})

	t.Run("errors.Join no match", func(t *testing.T) {
		err := std_errors.Join(
			std_errors.New("first"),
			std_errors.New("second"),
		)

		_, ok := errors.AsType[*testAppError](err)
		if ok {
			t.Fatal("expected no match in joined errors")
		}
	})

	t.Run("errors.Join with nil children", func(t *testing.T) {
		err := std_errors.Join(
			nil,
			&testAppError{Code: 200, Message: "ok"},
			nil,
		)

		target, ok := errors.AsType[*testAppError](err)
		if !ok {
			t.Fatal("expected match despite nil children")
		}
		if target.Code != 200 {
			t.Errorf("Code = %d, want 200", target.Code)
		}
	})

	t.Run("direct match without wrapping", func(t *testing.T) {
		err := &testAppError{Code: 418, Message: "teapot"}

		target, ok := errors.AsType[*testAppError](err)
		if !ok {
			t.Fatal("expected direct match")
		}
		if target.Code != 418 {
			t.Errorf("Code = %d, want 418", target.Code)
		}
	})
}

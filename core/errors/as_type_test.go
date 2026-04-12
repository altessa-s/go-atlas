// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"fmt"
	"io/fs"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

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
		require.True(t, ok, "expected match")
		require.Equal(t, 404, target.Code)
		require.Equal(t, "not found", target.Message)
	})

	t.Run("matches interface type", func(t *testing.T) {
		err := fmt.Errorf("wrapped: %w", &fs.PathError{Op: "open", Path: "/tmp/x", Err: os.ErrNotExist})

		target, ok := errors.AsType[*fs.PathError](err)
		require.True(t, ok, "expected match")
		require.Equal(t, "open", target.Op)
	})

	t.Run("no match returns zero value and false", func(t *testing.T) {
		err := std_errors.New("plain error")

		target, ok := errors.AsType[*testAppError](err)
		require.False(t, ok, "expected no match")
		require.Nil(t, target)
	})

	t.Run("nil error returns zero value and false", func(t *testing.T) {
		target, ok := errors.AsType[*testAppError](nil)
		require.False(t, ok, "expected no match for nil")
		require.Nil(t, target)
	})

	t.Run("deeply wrapped error", func(t *testing.T) {
		original := &testAppError{Code: 500, Message: "internal"}
		err := fmt.Errorf("level3: %w", fmt.Errorf("level2: %w", fmt.Errorf("level1: %w", original)))

		target, ok := errors.AsType[*testAppError](err)
		require.True(t, ok, "expected match through wrapping chain")
		require.Equal(t, 500, target.Code)
	})

	t.Run("custom As method", func(t *testing.T) {
		err := &customAsError{code: 42}

		target, ok := errors.AsType[*testAppError](err)
		require.True(t, ok, "expected match via As method")
		require.Equal(t, 42, target.Code)
		require.Equal(t, "from As", target.Message)
	})

	t.Run("custom As method wrapped", func(t *testing.T) {
		err := fmt.Errorf("outer: %w", &customAsError{code: 99})

		target, ok := errors.AsType[*testAppError](err)
		require.True(t, ok, "expected match via As method in chain")
		require.Equal(t, 99, target.Code)
	})

	t.Run("errors.Join multi-error", func(t *testing.T) {
		err := std_errors.Join(
			std_errors.New("first"),
			&testAppError{Code: 503, Message: "unavailable"},
			std_errors.New("third"),
		)

		target, ok := errors.AsType[*testAppError](err)
		require.True(t, ok, "expected match in joined errors")
		require.Equal(t, 503, target.Code)
	})

	t.Run("errors.Join no match", func(t *testing.T) {
		err := std_errors.Join(
			std_errors.New("first"),
			std_errors.New("second"),
		)

		_, ok := errors.AsType[*testAppError](err)
		require.False(t, ok, "expected no match in joined errors")
	})

	t.Run("errors.Join with nil children", func(t *testing.T) {
		err := std_errors.Join(
			nil,
			&testAppError{Code: 200, Message: "ok"},
			nil,
		)

		target, ok := errors.AsType[*testAppError](err)
		require.True(t, ok, "expected match despite nil children")
		require.Equal(t, 200, target.Code)
	})

	t.Run("direct match without wrapping", func(t *testing.T) {
		err := &testAppError{Code: 418, Message: "teapot"}

		target, ok := errors.AsType[*testAppError](err)
		require.True(t, ok, "expected direct match")
		require.Equal(t, 418, target.Code)
	})
}

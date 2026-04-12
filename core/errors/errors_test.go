// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/errors"

	std_errors "errors"
)

func TestContextChecks(t *testing.T) {
	t.Run("IsContextDeadlineExceeded", func(t *testing.T) {
		require.True(t, errors.IsContextDeadlineExceeded(context.DeadlineExceeded), "Should detect DeadlineExceeded")
		require.False(t, errors.IsContextDeadlineExceeded(std_errors.New("other")), "Should not detect other")
		require.False(t, errors.IsContextDeadlineExceeded(nil), "Should not detect nil")
	})

	t.Run("IsContextCanceled", func(t *testing.T) {
		require.True(t, errors.IsContextCanceled(context.Canceled), "Should detect Canceled")
	})

	t.Run("IsContextCanceledOrDeadlineExceeded", func(t *testing.T) {
		require.True(t, errors.IsContextCanceledOrDeadlineExceeded(context.Canceled), "Should detect Canceled")
		require.True(t, errors.IsContextCanceledOrDeadlineExceeded(context.DeadlineExceeded), "Should detect DeadlineExceeded")
	})
}

func TestWrappers(t *testing.T) {
	base := std_errors.New("base")

	t.Run("Wrapf", func(t *testing.T) {
		err := errors.Wrapf(base, "context %s", "foo")
		require.Equal(t, "context foo: base", err.Error())
		require.True(t, std_errors.Is(err, base), "Wrapf should wrap base")
		require.Nil(t, errors.Wrapf(nil, "foo"), "Wrapf(nil) should be nil")
	})

	t.Run("Wrap", func(t *testing.T) {
		err := errors.Wrap(base, "context")
		require.Equal(t, "context: base", err.Error())
	})

	t.Run("WrapOperation", func(t *testing.T) {
		err := errors.WrapOperation(base, "read")
		require.Equal(t, "failed to read: base", err.Error())
	})

	t.Run("WrapField", func(t *testing.T) {
		err := errors.WrapField(base, "name")
		require.Equal(t, "field 'name': base", err.Error())
	})

	t.Run("WrapOperationWithContext", func(t *testing.T) {
		err := errors.WrapOperationWithContext(base, "read", "file")
		require.Equal(t, "failed to read on file: base", err.Error())
	})

	t.Run("JoinWrap", func(t *testing.T) {
		sentinel := std_errors.New("sentinel")
		err := errors.JoinWrap(sentinel, base)
		require.Equal(t, "sentinel: base", err.Error())
		require.True(t, std_errors.Is(err, sentinel), "JoinWrap should wrap sentinel")
		require.True(t, std_errors.Is(err, base), "JoinWrap should wrap cause")
		require.Nil(t, errors.JoinWrap(sentinel, nil), "JoinWrap with nil cause should be nil")
		// Nil sentinel must return cause unchanged — without the
		// guard fmt.Errorf("%w: %w", nil, cause) produces a
		// malformed "%!w(<nil>): cause" message.
		require.Equal(t, base, errors.JoinWrap(nil, base), "JoinWrap(nil, base) should return base unchanged")
		require.Nil(t, errors.JoinWrap(nil, nil), "JoinWrap(nil, nil) should be nil")
	})
}

func TestStandardErrors(t *testing.T) {
	require.Equal(t, "db is required for app", errors.Required("db", "app").Error())
	base := std_errors.New("oops")
	require.Equal(t, "failed to create Redis provider: oops", errors.Provider("Redis", base).Error())
}

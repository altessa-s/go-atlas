// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/errors"

	std_errors "errors"
)

func TestCanonicalSentinelsAreDistinct(t *testing.T) {
	t.Parallel()

	sentinels := []error{
		errors.ErrNotFound, errors.ErrAlreadyExists, errors.ErrConflict, errors.ErrInvalidArgument,
		errors.ErrFailedPrecondition, errors.ErrUnavailable, errors.ErrBusy,
	}
	for i, a := range sentinels {
		for j, b := range sentinels {
			require.Equal(t, i == j, std_errors.Is(a, b), "%v vs %v", a, b)
		}
	}
	// The InvalidArgument text is published to clients by the gRPC mapping.
	require.Equal(t, "invalid argument", errors.ErrInvalidArgument.Error())
}

func TestFieldError(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("create: %w", errors.NewFieldError("attrs.law_code", "required"))
	require.ErrorIs(t, err, errors.ErrInvalidArgument)
	require.Equal(t, "create: invalid argument: attrs.law_code: required", err.Error())

	fe, ok := std_errors.AsType[*errors.FieldError](err)
	require.True(t, ok)
	require.Equal(t, "attrs.law_code", fe.Field)
	require.Equal(t, "required", fe.Reason)
}

func TestFieldErrorKeepsCause(t *testing.T) {
	t.Parallel()

	cause := std_errors.New("page token expired")
	err := fmt.Errorf("list: %w", &errors.FieldError{Field: "pageToken", Reason: "expired", Err: cause})
	require.ErrorIs(t, err, errors.ErrInvalidArgument)
	require.ErrorIs(t, err, cause)
	require.Equal(t, "list: invalid argument: pageToken: expired", err.Error(), "the cause is not part of the text")

	mapped := errors.MapFieldPath(err, func(f string) string { return "req." + f })
	fe, ok := std_errors.AsType[*errors.FieldError](mapped)
	require.True(t, ok)
	require.Equal(t, "req.pageToken", fe.Field)
	require.Same(t, cause, fe.Err, "the mapped field error keeps the cause")
}

func TestMapFieldPath(t *testing.T) {
	t.Parallel()

	prefix := func(p string) func(string) string { return func(f string) string { return p + "." + f } }

	t.Run("re-roots and keeps the chain", func(t *testing.T) {
		t.Parallel()
		orig := errors.NewFieldError("base_url", "required")
		err := fmt.Errorf("save: %w", std_errors.Join(errors.ErrConflict, orig))
		mapped := errors.MapFieldPath(err, prefix("provider"))

		fe, ok := std_errors.AsType[*errors.FieldError](mapped)
		require.True(t, ok)
		require.Equal(t, "provider.base_url", fe.Field)
		require.Equal(t, "required", fe.Reason)
		require.ErrorIs(t, mapped, errors.ErrInvalidArgument)
		require.ErrorIs(t, mapped, errors.ErrConflict, "the original chain stays matchable")
		require.ErrorIs(t, mapped, orig)
		require.Equal(t, err.Error(), mapped.Error(), "the text is the original one")
		require.Equal(t, "base_url", orig.(*errors.FieldError).Field, "the original is not modified") //nolint:errcheck,forcetypeassert // test
	})

	t.Run("mapping composes", func(t *testing.T) {
		t.Parallel()
		err := errors.MapFieldPath(errors.MapFieldPath(errors.NewFieldError("name", "required"), prefix("bot")), prefix("req"))
		fe, ok := std_errors.AsType[*errors.FieldError](err)
		require.True(t, ok)
		require.Equal(t, "req.bot.name", fe.Field)
	})

	t.Run("unchanged without a field error", func(t *testing.T) {
		t.Parallel()
		plain := fmt.Errorf("%w: bad cursor", errors.ErrInvalidArgument)
		require.Same(t, plain, errors.MapFieldPath(plain, prefix("x")))
		require.NoError(t, errors.MapFieldPath(nil, prefix("x")))
		fe := errors.NewFieldError("name", "required")
		require.Same(t, fe, errors.MapFieldPath(fe, nil))
	})
}

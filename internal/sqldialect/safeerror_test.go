// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type stateError struct{ text, state string }

func (e *stateError) Error() string    { return e.text }
func (e *stateError) SQLState() string { return e.state }

func TestSafeError_HidesTheDriverText(t *testing.T) {
	t.Parallel()

	for _, tc := range testhelpers.PostgresSecretDSNs() {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			driverErr := &stateError{text: fmt.Sprintf("cannot parse %q: %v", tc.DSN, tc.Secrets), state: "08001"}
			err := sqldialect.SafeError(nil, "connect", fmt.Errorf("open: %w", driverErr))
			for _, s := range tc.Secrets {
				require.False(t, strings.Contains(err.Error(), s), "password in diagnostic")
				require.False(t, strings.Contains(fmt.Sprintf("%+v", err), s), "password in formatted diagnostic")
			}
			require.Contains(t, err.Error(), "connect failed")
			require.Contains(t, err.Error(), "SQLSTATE 08001")
			require.True(t, errors.Unwrap(err) == nil, "the original error is never reachable")
			_, ok := errors.AsType[*stateError](err)
			require.False(t, ok, "errors.As does not reach the driver error")
		})
	}
}

func TestSafeError_KeepsClassification(t *testing.T) {
	t.Parallel()

	errKind := errors.New("kind")
	err := sqldialect.SafeError(errKind, "ping", fmt.Errorf("dial: %w", context.DeadlineExceeded))
	require.ErrorIs(t, err, errKind)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, context.Canceled)
	require.Equal(t, "ping failed (*fmt.wrapError)", err.Error())
	require.NoError(t, sqldialect.SafeError(errKind, "ping", nil))
}

// SQLState() of the driver error is a driver-controlled string; only a well-formed five-character code
// is kept, and it stays available through SQLState() for classification.
func TestSafeError_KeepsOnlyAValidSQLState(t *testing.T) {
	t.Parallel()

	tests := []struct{ state, want string }{
		{"23505", "23505"},
		{"40P01", "40P01"},
		{"s3cret-password", ""},
		{"4000", ""},
		{"23505 ", ""},
		{"ab123", ""},
		{"", ""},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			err := sqldialect.SafeError(nil, "migrate", fmt.Errorf("step: %w", &stateError{text: "x", state: tc.state}))
			require.NotContains(t, err.Error(), "s3cret")
			got, ok := errors.AsType[interface {
				error
				SQLState() string
			}](err)
			require.True(t, ok, "the result exposes SQLState")
			require.Equal(t, tc.want, got.SQLState(), "state %q", tc.state)
			require.Equal(t, tc.want, sqldialect.SQLState(err))
			if tc.want != "" {
				require.Contains(t, err.Error(), "SQLSTATE "+tc.want)
			} else {
				require.NotContains(t, err.Error(), "SQLSTATE")
			}
		})
	}
}

// probingError claims to match every target through a custom Is, as an error built to probe values would.
type probingError struct{}

func (probingError) Error() string        { return "probe" }
func (probingError) Is(target error) bool { return true }

// Errors.Is on the result matches its kind and an enumerated set of sentinels by identity only; the
// original error's own Is is never consulted.
func TestSafeError_MatchesOnlyEnumeratedSentinelsByIdentity(t *testing.T) {
	t.Parallel()

	errKind := errors.New("kind")
	errOther := errors.New("other")
	err := sqldialect.SafeError(errKind, "ping", probingError{})
	require.ErrorIs(t, err, errKind)
	require.NotErrorIs(t, err, errOther, "a custom Is of the original error is not consulted")
	require.NotErrorIs(t, err, context.Canceled, "identity only, even for enumerated sentinels")

	err = sqldialect.SafeError(nil, "ping", errors.Join(errOther, fmt.Errorf("dial: %w", context.Canceled)))
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, errOther, "a sentinel that is not enumerated never matches")
}

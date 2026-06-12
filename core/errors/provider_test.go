// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errors_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/errors"

	std_errors "errors"
)

// TestProvider_NilReturnsNil asserts that Provider, like every sibling Wrap*
// helper in this package, is safe to call with a nil error: it must return nil
// rather than producing a malformed "failed to create X provider: %!w(<nil>)"
// message.
//
// Regression for the missing nil guard previously present in Provider.
func TestProvider_NilReturnsNil(t *testing.T) {
	require.Nil(t, errors.Provider("Redis", nil), "Provider(_, nil) should be nil")
}

// TestProvider_WrapsCauseAndPreservesChain asserts that a non-nil cause is
// wrapped with the %w verb so errors.Is still matches the original cause.
func TestProvider_WrapsCauseAndPreservesChain(t *testing.T) {
	cause := std_errors.New("connection refused")

	err := errors.Provider("Redis", cause)
	require.NotNil(t, err, "Provider(_, cause) should not be nil")
	require.True(t, std_errors.Is(err, cause), "errors.Is(err, cause) = false, want true (chain broken)")

	const want = "failed to create Redis provider: connection refused"
	require.Equal(t, want, err.Error())
}

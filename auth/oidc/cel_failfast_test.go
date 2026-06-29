// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCompileCELRulesFailsFast verifies a malformed programmatic CEL rule is a
// hard error rather than a silently-skipped rule (which would weaken
// validation), matching the strict service-config loader.
func TestCompileCELRulesFailsFast(t *testing.T) {
	t.Parallel()

	// A valid rule compiles.
	compiled, err := compileCELRules([]CELValidationRule{{Name: "ok", Expression: `"a" == "a"`}})
	require.NoError(t, err)
	require.Len(t, compiled, 1)

	// An expression that fails to compile is a hard error.
	_, err = compileCELRules([]CELValidationRule{{Name: "bad", Expression: "this is not CEL ++"}})
	require.ErrorIs(t, err, ErrInvalidCEL)

	// Empty name and empty expression are errors too.
	_, err = compileCELRules([]CELValidationRule{{Name: "", Expression: `"a" == "a"`}})
	require.ErrorIs(t, err, ErrInvalidCEL)
	_, err = compileCELRules([]CELValidationRule{{Name: "x", Expression: ""}})
	require.ErrorIs(t, err, ErrInvalidCEL)
}

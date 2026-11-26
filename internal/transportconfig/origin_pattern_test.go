// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package transportconfig

import (
	"regexp"
	"regexp/syntax"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompileOriginPattern(t *testing.T) {
	t.Parallel()

	_, err := CompileOriginPattern("[", false)
	var syntaxErr *syntax.Error
	require.ErrorAs(t, err, &syntaxErr)

	_, err = CompileOriginPattern(`^https://example\.com\..*$`, true)
	require.ErrorIs(t, err, ErrCredentialedOriginWildcard)
}

func TestValidateCredentialedOriginPattern(t *testing.T) {
	t.Parallel()

	require.ErrorIs(t, ValidateCredentialedOriginPattern(nil), ErrCredentialedOriginWildcard)
	require.NoError(t, ValidateCredentialedOriginPattern(regexp.MustCompile(`^https://.*\.city\.kawasaki\.jp$`)))
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rules_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/auth/oidc/rules"
)

func TestCheckLeeway(t *testing.T) {
	t.Parallel()
	require.NoError(t, rules.CheckLeeway(jwt.MaxLeeway))
	require.ErrorIs(t, rules.CheckLeeway(jwt.MaxLeeway+time.Nanosecond), rules.ErrInvalidConfig)
}

func TestCompileCEL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, expression string
		valid            bool
	}{
		{"boolean", "claims.sub == 'reader'", true},
		{"string extension", "claims.scope.split(' ').exists(s, s == 'read')", true},
		{"nonboolean", "claims.sub + 'x'", false},
		{"oversized", strings.Repeat(" ", rules.DefaultCELMaxExpressionLength) + "true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			program, err := rules.CompileCEL(tc.expression)
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, program)
				return
			}
			require.Error(t, err)
			require.Nil(t, program)
		})
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rules_test

import (
	"encoding/json"
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

func TestCELNumericClaims(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, expression string
		claims           map[string]any
		want             bool
	}{
		{"float level", "claims.level == 3", map[string]any{"level": float64(3)}, true},
		{"json.Number level", "claims.level == 3", map[string]any{"level": json.Number("3")}, true},
		{"float differs", "claims.level == 4", map[string]any{"level": float64(3)}, false},
		{"big number matches itself", "claims.tenant == 9007199254740993", map[string]any{"tenant": json.Number("9007199254740993")}, true},
		{"big number not its neighbor", "claims.tenant == 9007199254740992", map[string]any{"tenant": json.Number("9007199254740993")}, false},
		{"big uint64", "claims.tenant == 18446744073709551615u", map[string]any{"tenant": json.Number("18446744073709551615")}, true},
		{"nested big number", "claims.org.id == 9007199254740993", map[string]any{"org": map[string]any{"id": json.Number("9007199254740993")}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			eval, err := rules.CompileCEL(tc.expression)
			require.NoError(t, err)
			require.Equal(t, tc.want, eval(tc.claims))
		})
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/auth/oidc/rules"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

// TestConditionsToMatcher pins how every JSON preset-rule condition is
// compiled and evaluated.
func TestConditionsToMatcher(t *testing.T) {
	t.Parallel()

	claims := map[string]any{
		"sub":    "user-1",
		"tier":   float64(2),
		"admin":  true,
		"nil":    nil,
		"email":  "a@example.com",
		"groups": []any{"x", "y"},
		"scope":  "read write",
	}

	tests := []struct {
		name       string
		conditions map[string]any
		want       bool
	}{
		{name: "claim_equals string", conditions: map[string]any{"claim_equals": map[string]any{"sub": "user-1"}}, want: true},
		{name: "claim_equals string mismatch", conditions: map[string]any{"claim_equals": map[string]any{"sub": "user-2"}}},
		{name: "claim_equals number", conditions: map[string]any{"claim_equals": map[string]any{"tier": float64(2)}}, want: true},
		{name: "claim_equals number vs string", conditions: map[string]any{"claim_equals": map[string]any{"sub": float64(1)}}},
		{name: "claim_equals bool", conditions: map[string]any{"claim_equals": map[string]any{"admin": true}}, want: true},
		{name: "claim_equals null", conditions: map[string]any{"claim_equals": map[string]any{"nil": nil}}, want: true},
		{name: "claim_equals missing claim", conditions: map[string]any{"claim_equals": map[string]any{"absent": "x"}}},
		{name: "claim_equals other type by text", conditions: map[string]any{"claim_equals": map[string]any{"groups": []any{"x", "y"}}}, want: true},
		{name: "claim_equals all keys", conditions: map[string]any{"claim_equals": map[string]any{"sub": "user-1", "admin": false}}},
		{name: "claim_exists", conditions: map[string]any{"claim_exists": []any{"sub", "email"}}, want: true},
		{name: "claim_exists missing", conditions: map[string]any{"claim_exists": []any{"sub", "absent"}}},
		{name: "claim_contains", conditions: map[string]any{"claim_contains": map[string]any{"email": "@example.com"}}, want: true},
		{name: "claim_contains miss", conditions: map[string]any{"claim_contains": map[string]any{"email": "@other"}}},
		{name: "has_scope", conditions: map[string]any{"has_scope": "write"}, want: true},
		{name: "has_scope miss", conditions: map[string]any{"has_scope": "admin"}},
		{name: "has_any_scope", conditions: map[string]any{"has_any_scope": []any{"admin", "read"}}, want: true},
		{name: "has_all_scopes", conditions: map[string]any{"has_all_scopes": []any{"read", "write"}}, want: true},
		{name: "has_all_scopes miss", conditions: map[string]any{"has_all_scopes": []any{"read", "admin"}}},
		{name: "cel_expression", conditions: map[string]any{"cel_expression": "claims.sub == 'user-1'"}, want: true},
		{name: "cel_expression false", conditions: map[string]any{"cel_expression": "claims.sub == 'other'"}},
		{name: "matcher_and", conditions: map[string]any{"matcher_and": []any{
			map[string]any{"has_scope": "read"}, map[string]any{"claim_equals": map[string]any{"admin": true}},
		}}, want: true},
		{name: "matcher_and one false", conditions: map[string]any{"matcher_and": []any{
			map[string]any{"has_scope": "read"}, map[string]any{"has_scope": "admin"},
		}}},
		{name: "matcher_or", conditions: map[string]any{"matcher_or": []any{
			map[string]any{"has_scope": "admin"}, map[string]any{"has_scope": "read"},
		}}, want: true},
		{name: "matcher_or all false", conditions: map[string]any{"matcher_or": []any{
			map[string]any{"has_scope": "admin"}, map[string]any{"has_scope": "root"},
		}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			matcher, err := presetMatcher(&authconfig.PresetRuleDefinition{Conditions: tc.conditions})
			require.NoError(t, err)
			require.Equal(t, tc.want, matcher(claims))
		})
	}
}

func TestConditionsToMatcher_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		conditions map[string]any
		wantErr    error
		contains   string
	}{
		{name: "empty", conditions: map[string]any{}, wantErr: rules.ErrInvalidConfig},
		{name: "two matchers", conditions: map[string]any{"has_scope": "a", "claim_exists": []any{"b"}}, wantErr: rules.ErrInvalidConfig},
		{name: "unknown type", conditions: map[string]any{"bogus": 1}, wantErr: rules.ErrInvalidConfig, contains: `unknown condition type "bogus"`},
		{name: "claim_equals not object", conditions: map[string]any{"claim_equals": "x"}, wantErr: rules.ErrInvalidConfig},
		{name: "claim_equals empty", conditions: map[string]any{"claim_equals": map[string]any{}}, wantErr: rules.ErrInvalidConfig},
		{name: "claim_exists not array", conditions: map[string]any{"claim_exists": "x"}, wantErr: rules.ErrInvalidConfig},
		{name: "claim_exists blank name", conditions: map[string]any{"claim_exists": []any{" "}}, wantErr: rules.ErrInvalidConfig},
		{name: "claim_exists non-string", conditions: map[string]any{"claim_exists": []any{1}}, wantErr: rules.ErrInvalidConfig},
		{name: "claim_contains not object", conditions: map[string]any{"claim_contains": []any{}}, wantErr: rules.ErrInvalidConfig},
		{name: "claim_contains non-string value", conditions: map[string]any{"claim_contains": map[string]any{"a": 1}}, wantErr: rules.ErrInvalidConfig},
		{name: "has_scope blank", conditions: map[string]any{"has_scope": " "}, wantErr: rules.ErrInvalidConfig},
		{name: "has_scope non-string", conditions: map[string]any{"has_scope": 1}, wantErr: rules.ErrInvalidConfig},
		{name: "has_any_scope empty", conditions: map[string]any{"has_any_scope": []any{}}, wantErr: rules.ErrInvalidConfig},
		{name: "has_all_scopes non-string", conditions: map[string]any{"has_all_scopes": []any{1}}, wantErr: rules.ErrInvalidConfig},
		{name: "cel_expression blank", conditions: map[string]any{"cel_expression": ""}, wantErr: rules.ErrInvalidConfig},
		{name: "cel_expression invalid", conditions: map[string]any{"cel_expression": "claims.("}, wantErr: rules.ErrInvalidCEL},
		{name: "matcher_and empty", conditions: map[string]any{"matcher_and": []any{}}, wantErr: rules.ErrInvalidConfig, contains: "matcher_and requires"},
		{name: "matcher_or element not object", conditions: map[string]any{"matcher_or": []any{"x"}}, wantErr: rules.ErrInvalidConfig, contains: "matcher_or[0]"},
		{name: "matcher_and nested error", conditions: map[string]any{"matcher_and": []any{
			map[string]any{"has_scope": "a"}, map[string]any{"bogus": 1},
		}}, wantErr: rules.ErrInvalidConfig, contains: "matcher_and[1]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := presetMatcher(&authconfig.PresetRuleDefinition{Conditions: tc.conditions})
			require.ErrorIs(t, err, tc.wantErr)
			if tc.contains != "" {
				require.ErrorContains(t, err, tc.contains)
			}
		})
	}
}

// Parsing a CEL preset prepares the matcher shared with programmatic rules. An
// already canceled context can use that compiled entry but cannot compile a miss.
func TestParsedPresetSharesCELMatcher(t *testing.T) {
	t.Parallel()
	subject := rand.Text()
	expression := fmt.Sprintf("claims.sub == %q", subject)
	doc := fmt.Sprintf(`{"version":"1.0","presets":[{"name":"p","validation":{}}],
"preset_rules":[{"preset":"p","conditions":{"cel_expression":%q}}]}`, expression)
	_, err := authconfig.ParseServiceConfig([]byte(doc))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.True(t, oidc.CELMatcher(ctx, expression)(map[string]any{"sub": subject}))
}

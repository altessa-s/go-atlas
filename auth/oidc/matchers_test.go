// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaimEquals(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		claim  string
		value  string
		want   bool
	}{
		{"match", map[string]any{"role": "admin"}, "role", "admin", true},
		{"no match", map[string]any{"role": "user"}, "role", "admin", false},
		{"missing claim", map[string]any{}, "role", "admin", false},
		{"non-string value", map[string]any{"role": 123}, "role", "admin", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ClaimEquals(tt.claim, tt.value)(tt.claims))
		})
	}
}

func TestClaimContains(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		want   bool
	}{
		{"contains", map[string]any{"realm": "one2work-realm"}, true},
		{"not contains", map[string]any{"realm": "other"}, false},
		{"missing", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ClaimContains("realm", "one2work")(tt.claims))
		})
	}
}

func TestClaimExists(t *testing.T) {
	require.True(t, ClaimExists("email")(map[string]any{"email": "a@b.com"}))
	require.False(t, ClaimExists("email")(map[string]any{}))
}

func TestHasScope(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		scope  string
		want   bool
	}{
		{"string scope match", map[string]any{"scope": "read write"}, "read", true},
		{"string scope no match", map[string]any{"scope": "read write"}, "admin", false},
		{"array scope", map[string]any{"scope": []any{"read", "write"}}, "write", true},
		{"no scope claim", map[string]any{}, "read", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, HasScope(tt.scope)(tt.claims))
		})
	}
}

func TestHasAnyScope(t *testing.T) {
	claims := map[string]any{"scope": "read write"}
	require.True(t, HasAnyScope("admin", "read")(claims))
	require.False(t, HasAnyScope("admin", "delete")(claims))
	// Test with >3 scopes to hit map path
	require.True(t, HasAnyScope("a", "b", "c", "read")(claims))
}

func TestHasAllScopes(t *testing.T) {
	claims := map[string]any{"scope": "read write admin"}
	require.True(t, HasAllScopes("read", "write")(claims))
	require.False(t, HasAllScopes("read", "delete")(claims))
}

func TestMatcherAnd(t *testing.T) {
	claims := map[string]any{"role": "admin", "scope": "read"}
	m := MatcherAnd(ClaimEquals("role", "admin"), HasScope("read"))
	require.True(t, m(claims))
	m2 := MatcherAnd(ClaimEquals("role", "admin"), HasScope("write"))
	require.False(t, m2(claims))
}

func TestMatcherOr(t *testing.T) {
	claims := map[string]any{"role": "user"}
	m := MatcherOr(ClaimEquals("role", "admin"), ClaimEquals("role", "user"))
	require.True(t, m(claims))
	m2 := MatcherOr(ClaimEquals("role", "admin"), ClaimEquals("role", "mod"))
	require.False(t, m2(claims))
}

func TestMatcherNot(t *testing.T) {
	claims := map[string]any{"role": "user"}
	require.True(t, MatcherNot(ClaimEquals("role", "admin"))(claims))
	require.False(t, MatcherNot(ClaimEquals("role", "user"))(claims))
}

func TestClientIDEquals(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		want   bool
	}{
		{"client_id match", map[string]any{"client_id": "svc"}, true},
		{"azp fallback", map[string]any{"azp": "svc"}, true},
		{"no match", map[string]any{"client_id": "other"}, false},
		{"missing both", map[string]any{}, false},
		{"non-string", map[string]any{"client_id": 123}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ClientIDEquals("svc")(tt.claims))
		})
	}
}

func TestIssuerEquals(t *testing.T) {
	claims := map[string]any{"iss": "https://auth.example.com"}
	require.True(t, IssuerEquals("https://auth.example.com")(claims))
	require.False(t, IssuerEquals("other")(claims))
}

func TestAudienceContains(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		want   bool
	}{
		{"string aud match", map[string]any{"aud": "api.example.com"}, true},
		{"string aud no match", map[string]any{"aud": "other"}, false},
		{"array aud match", map[string]any{"aud": []any{"api.example.com", "other"}}, true},
		{"array aud no match", map[string]any{"aud": []any{"other"}}, false},
		{"missing", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, AudienceContains("api.example.com")(tt.claims))
		})
	}
}

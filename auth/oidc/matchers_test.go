// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import "testing"

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
			if got := ClaimEquals(tt.claim, tt.value)(tt.claims); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
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
			if got := ClaimContains("realm", "one2work")(tt.claims); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClaimExists(t *testing.T) {
	if !ClaimExists("email")(map[string]any{"email": "a@b.com"}) {
		t.Error("expected true")
	}
	if ClaimExists("email")(map[string]any{}) {
		t.Error("expected false")
	}
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
			if got := HasScope(tt.scope)(tt.claims); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasAnyScope(t *testing.T) {
	claims := map[string]any{"scope": "read write"}
	if !HasAnyScope("admin", "read")(claims) {
		t.Error("expected true")
	}
	if HasAnyScope("admin", "delete")(claims) {
		t.Error("expected false")
	}
	// Test with >3 scopes to hit map path
	if !HasAnyScope("a", "b", "c", "read")(claims) {
		t.Error("expected true for large set")
	}
}

func TestHasAllScopes(t *testing.T) {
	claims := map[string]any{"scope": "read write admin"}
	if !HasAllScopes("read", "write")(claims) {
		t.Error("expected true")
	}
	if HasAllScopes("read", "delete")(claims) {
		t.Error("expected false")
	}
}

func TestMatcherAnd(t *testing.T) {
	claims := map[string]any{"role": "admin", "scope": "read"}
	m := MatcherAnd(ClaimEquals("role", "admin"), HasScope("read"))
	if !m(claims) {
		t.Error("expected true")
	}
	m2 := MatcherAnd(ClaimEquals("role", "admin"), HasScope("write"))
	if m2(claims) {
		t.Error("expected false")
	}
}

func TestMatcherOr(t *testing.T) {
	claims := map[string]any{"role": "user"}
	m := MatcherOr(ClaimEquals("role", "admin"), ClaimEquals("role", "user"))
	if !m(claims) {
		t.Error("expected true")
	}
	m2 := MatcherOr(ClaimEquals("role", "admin"), ClaimEquals("role", "mod"))
	if m2(claims) {
		t.Error("expected false")
	}
}

func TestMatcherNot(t *testing.T) {
	claims := map[string]any{"role": "user"}
	if !MatcherNot(ClaimEquals("role", "admin"))(claims) {
		t.Error("expected true")
	}
	if MatcherNot(ClaimEquals("role", "user"))(claims) {
		t.Error("expected false")
	}
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
			if got := ClientIDEquals("svc")(tt.claims); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIssuerEquals(t *testing.T) {
	claims := map[string]any{"iss": "https://auth.example.com"}
	if !IssuerEquals("https://auth.example.com")(claims) {
		t.Error("expected true")
	}
	if IssuerEquals("other")(claims) {
		t.Error("expected false")
	}
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
			if got := AudienceContains("api.example.com")(tt.claims); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

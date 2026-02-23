// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validator

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStringClaim(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		key    string
		want   string
	}{
		{"string", map[string]any{"sub": "user1"}, "sub", "user1"},
		{"missing", map[string]any{}, "sub", ""},
		{"empty", map[string]any{"sub": ""}, "sub", ""},
		{"non_string", map[string]any{"sub": 123}, "sub", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stringClaim(tt.claims, tt.key); got != tt.want {
				t.Fatalf("stringClaim() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAudienceClaim(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		want   int
	}{
		{"string", map[string]any{"aud": "client1"}, 1},
		{"string_slice", map[string]any{"aud": []string{"a", "b"}}, 2},
		{"any_slice", map[string]any{"aud": []any{"a", "b"}}, 2},
		{"missing", map[string]any{}, 0},
		{"empty_string", map[string]any{"aud": ""}, 0},
		{"empty_slice", map[string]any{"aud": []string{}}, 0},
		{"any_slice_non_string", map[string]any{"aud": []any{123}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := audienceClaim(tt.claims, "aud")
			if len(got) != tt.want {
				t.Fatalf("audienceClaim() len = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestScopesClaim(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		want   int
	}{
		{"space_separated", map[string]any{"scopes": "openid profile"}, 2},
		{"string_slice", map[string]any{"scopes": []string{"a", "b", "c"}}, 3},
		{"any_slice", map[string]any{"scopes": []any{"x", "y"}}, 2},
		{"missing", map[string]any{}, 0},
		{"empty_string", map[string]any{"scopes": ""}, 0},
		{"filtered_empty", map[string]any{"scopes": []string{"", ""}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scopesClaim(tt.claims, "scopes")
			if len(got) != tt.want {
				t.Fatalf("scopesClaim() len = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestScopesClaim_Sorted(t *testing.T) {
	got := scopesClaim(map[string]any{"scopes": "z a m"}, "scopes")
	if len(got) != 3 || got[0] != "a" || got[1] != "m" || got[2] != "z" {
		t.Fatalf("scopes not sorted: %v", got)
	}
}

func TestUnixTimeClaim(t *testing.T) {
	ts := int64(1700000000)
	expected := time.Unix(ts, 0).UTC()

	tests := []struct {
		name   string
		claims map[string]any
		isZero bool
	}{
		{"float64", map[string]any{"exp": float64(ts)}, false},
		{"int", map[string]any{"exp": int(ts)}, false},
		{"int64", map[string]any{"exp": ts}, false},
		{"json_number", map[string]any{"exp": json.Number("1700000000")}, false},
		{"missing", map[string]any{}, true},
		{"invalid_json_number", map[string]any{"exp": json.Number("abc")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unixTimeClaim(tt.claims, "exp")
			if tt.isZero {
				if !got.IsZero() {
					t.Fatalf("expected zero time, got %v", got)
				}
			} else {
				if got != expected {
					t.Fatalf("got %v, want %v", got, expected)
				}
			}
		})
	}
}

func TestExtractClaims(t *testing.T) {
	raw := map[string]any{
		"sub":                "user1",
		"preferred_username": "jdoe",
		"email":              "j@d.com",
		"iss":                "https://issuer",
		"aud":                "client1",
		"scopes":             "openid profile",
		"exp":                float64(1700000000),
		"iat":                float64(1699999000),
		"name":               "John Doe",
		"given_name":         "John",
		"family_name":        "Doe",
	}
	claims := extractClaims(raw)

	if claims.Subject != "user1" {
		t.Fatalf("Subject = %q", claims.Subject)
	}
	if claims.Email != "j@d.com" {
		t.Fatalf("Email = %q", claims.Email)
	}
	if len(claims.Scopes) != 2 {
		t.Fatalf("Scopes len = %d", len(claims.Scopes))
	}
	if claims.RawClaims == nil {
		t.Fatal("RawClaims should not be nil")
	}
}

func BenchmarkExtractClaims(b *testing.B) {
	raw := map[string]any{
		"sub":    "user1",
		"iss":    "https://issuer",
		"aud":    "client1",
		"scopes": "openid profile",
		"exp":    float64(1700000000),
	}
	for b.Loop() {
		extractClaims(raw)
	}
}

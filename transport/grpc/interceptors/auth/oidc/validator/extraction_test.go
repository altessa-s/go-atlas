// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validator

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
			got := stringClaim(tt.claims, tt.key)
			require.Equal(t, tt.want, got)
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
			require.Equal(t, tt.want, len(got))
		})
	}
}

func TestScopesClaim(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		want   int
	}{
		{"space_separated", map[string]any{"scope": "openid profile"}, 2},
		{"string_slice", map[string]any{"scope": []string{"a", "b", "c"}}, 3},
		{"any_slice", map[string]any{"scope": []any{"x", "y"}}, 2},
		{"missing", map[string]any{}, 0},
		{"empty_string", map[string]any{"scope": ""}, 0},
		{"filtered_empty", map[string]any{"scope": []string{"", ""}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scopesClaim(tt.claims)
			require.Equal(t, tt.want, len(got))
		})
	}
}

func TestScopesClaim_Sorted(t *testing.T) {
	got := scopesClaim(map[string]any{"scope": "z a m"})
	require.Len(t, got, 3)
	require.Equal(t, "a", got[0])
	require.Equal(t, "m", got[1])
	require.Equal(t, "z", got[2])
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
				require.True(t, got.IsZero(), "expected zero time, got %v", got)
			} else {
				require.Equal(t, expected, got)
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
		"scope":              "openid profile",
		"exp":                float64(1700000000),
		"iat":                float64(1699999000),
		"name":               "John Doe",
		"given_name":         "John",
		"family_name":        "Doe",
	}
	claims := extractClaims(raw)

	require.Equal(t, "user1", claims.Subject)
	require.Equal(t, "j@d.com", claims.Email)
	require.Len(t, claims.Scopes, 2)
	require.NotNil(t, claims.RawClaims, "RawClaims should not be nil")
}

func BenchmarkExtractClaims(b *testing.B) {
	raw := map[string]any{
		"sub":   "user1",
		"iss":   "https://issuer",
		"aud":   "client1",
		"scope": "openid profile",
		"exp":   float64(1700000000),
	}
	for b.Loop() {
		extractClaims(raw)
	}
}

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

func TestExtractClaims_StringFields(t *testing.T) {
	c := extractClaims(map[string]any{
		"sub":                "user1",
		"preferred_username": "jdoe",
		"email":              "j@d.com",
		"iss":                "https://issuer",
		"name":               "John Doe",
		"given_name":         "John",
		"family_name":        "Doe",
	})
	require.Equal(t, "user1", c.Subject)
	require.Equal(t, "jdoe", c.PreferredUsername)
	require.Equal(t, "j@d.com", c.Email)
	require.Equal(t, "https://issuer", c.Issuer)
	require.Equal(t, "John Doe", c.Name)
	require.Equal(t, "John", c.GivenName)
	require.Equal(t, "Doe", c.FamilyName)
}

func TestExtractClaims_StringFieldMissingOrWrongType(t *testing.T) {
	c := extractClaims(map[string]any{"sub": 123})
	require.Empty(t, c.Subject, "a non-string claim coerces to empty")
	require.Empty(t, c.Email, "a missing claim is empty")
}

func TestExtractClaims_Audience(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want int
	}{
		{"string", map[string]any{"aud": "client1"}, 1},
		{"string_slice", map[string]any{"aud": []string{"a", "b"}}, 2},
		{"any_slice", map[string]any{"aud": []any{"a", "b"}}, 2},
		{"any_slice_non_string", map[string]any{"aud": []any{123}}, 0},
		{"missing", map[string]any{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Len(t, extractClaims(tt.raw).Audience, tt.want)
		})
	}
}

func TestExtractClaims_Scopes(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want int
	}{
		{"space_separated", map[string]any{"scope": "openid profile"}, 2},
		{"string_slice", map[string]any{"scope": []string{"a", "b", "c"}}, 3},
		{"any_slice", map[string]any{"scope": []any{"x", "y"}}, 2},
		{"missing", map[string]any{}, 0},
		{"empty_string", map[string]any{"scope": ""}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Len(t, extractClaims(tt.raw).Scopes, tt.want)
		})
	}
}

// TestExtractClaims_ScopesPreserveOrder documents that scopes keep their token
// order: the shared auth/jwt accessor does not sort, unlike the previous
// hand-rolled extractor.
func TestExtractClaims_ScopesPreserveOrder(t *testing.T) {
	got := extractClaims(map[string]any{"scope": "z a m"}).Scopes
	require.Equal(t, []string{"z", "a", "m"}, got)
}

func TestExtractClaims_NumericDates(t *testing.T) {
	ts := int64(1700000000)
	want := time.Unix(ts, 0).UTC()
	tests := []struct {
		name string
		raw  map[string]any
		zero bool
	}{
		{"float64", map[string]any{"exp": float64(ts)}, false},
		{"json_number", map[string]any{"exp": json.Number("1700000000")}, false},
		{"missing", map[string]any{}, true},
		{"invalid_json_number", map[string]any{"exp": json.Number("abc")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractClaims(tt.raw).ExpiresAt
			if tt.zero {
				require.True(t, got.IsZero(), "expected zero time, got %v", got)
			} else {
				require.Equal(t, want, got)
			}
		})
	}
}

func TestExtractClaims_Full(t *testing.T) {
	raw := map[string]any{
		"sub":   "user1",
		"email": "j@d.com",
		"aud":   "client1",
		"scope": "openid profile",
		"exp":   float64(1700000000),
		"iat":   float64(1699999000),
	}
	c := extractClaims(raw)

	require.Equal(t, "user1", c.Subject)
	require.Len(t, c.Scopes, 2)
	require.NotNil(t, c.RawClaims, "RawClaims should not be nil")
}

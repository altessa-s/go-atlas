// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validator

import "testing"

// BenchmarkDefaultValidator_ValidateToken measures the validator happy path
// with a minimal claim set.
func BenchmarkDefaultValidator_ValidateToken(b *testing.B) {
	p := &mockProvider{claims: map[string]any{"sub": "user1"}}
	v := NewDefaultValidator(p)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		v.ValidateToken(ctx, "tok") //nolint:errcheck
	}
}

// BenchmarkExtractClaims measures raw claim-map extraction into the typed
// claims struct.
func BenchmarkExtractClaims(b *testing.B) {
	raw := map[string]any{
		"sub":   "user1",
		"iss":   "https://issuer",
		"aud":   "client1",
		"scope": "openid profile",
		"exp":   float64(1700000000),
	}
	b.ReportAllocs()
	for b.Loop() {
		extractClaims(raw)
	}
}

// BenchmarkDefaultValidator_ValidateToken_FullClaims measures the validator
// happy path with a representative full OIDC claim set, exercising the claim
// extraction paths (audience array, scope string, numeric dates). It reuses
// the mockProvider fixture from validator_test.go.
func BenchmarkDefaultValidator_ValidateToken_FullClaims(b *testing.B) {
	p := &mockProvider{claims: map[string]any{
		"sub":                "user-12345",
		"iss":                "https://issuer.example.com",
		"aud":                []any{"api://default", "api://reports"},
		"scope":              "openid profile email offline_access",
		"exp":                float64(4102444800),
		"iat":                float64(1700000000),
		"nbf":                float64(1700000000),
		"preferred_username": "user@example.com",
		"email":              "user@example.com",
		"name":               "User Example",
		"given_name":         "User",
		"family_name":        "Example",
	}}
	v := NewDefaultValidator(p)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := v.ValidateToken(ctx, "token"); err != nil {
			b.Fatal(err)
		}
	}
}

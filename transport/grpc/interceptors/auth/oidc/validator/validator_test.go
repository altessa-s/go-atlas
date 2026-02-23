// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validator

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockProvider struct {
	claims map[string]any
	err    error
}

func (m *mockProvider) ValidateToken(_ context.Context, _ string) (map[string]any, error) {
	return m.claims, m.err
}

func TestDefaultValidator_ValidateToken_Success(t *testing.T) {
	p := &mockProvider{claims: map[string]any{"sub": "user1", "iss": "https://issuer"}}
	v := NewDefaultValidator(p)

	claims, err := v.ValidateToken(t.Context(), "valid-token")
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user1" {
		t.Fatalf("Subject = %q", claims.Subject)
	}
	if claims.Issuer != "https://issuer" {
		t.Fatalf("Issuer = %q", claims.Issuer)
	}
}

func TestDefaultValidator_ValidateToken_Error(t *testing.T) {
	p := &mockProvider{err: status.Error(codes.Unauthenticated, "bad")}
	v := NewDefaultValidator(p)

	_, err := v.ValidateToken(t.Context(), "bad-token")
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}
}

func BenchmarkDefaultValidator_ValidateToken(b *testing.B) {
	p := &mockProvider{claims: map[string]any{"sub": "user1"}}
	v := NewDefaultValidator(p)
	ctx := b.Context()
	for b.Loop() {
		v.ValidateToken(ctx, "tok") //nolint:errcheck
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockValidator struct {
	claims map[string]any
	err    error
}

func (m *mockValidator) ValidateToken(_ context.Context, _ string) (map[string]any, error) {
	return m.claims, m.err
}

func TestAuthFunc_Success(t *testing.T) {
	v := &mockValidator{claims: map[string]any{"sub": "user1"}}
	fn := AuthFunc(v)

	req := auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "valid-token"},
	}

	result, err := fn(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	claims, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map[string]any")
	}
	if claims["sub"] != "user1" {
		t.Fatalf("sub = %v", claims["sub"])
	}
}

func TestAuthFunc_MissingToken(t *testing.T) {
	v := &mockValidator{}
	fn := AuthFunc(v)

	req := auth.Request{Base: auth.Base{AuthMethod: "other"}}

	_, err := fn(t.Context(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}
}

func TestAuthFunc_ValidationError(t *testing.T) {
	v := &mockValidator{err: status.Error(codes.Unauthenticated, "bad token")}
	fn := AuthFunc(v)

	req := auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "bad"},
	}

	_, err := fn(t.Context(), req)
	if err == nil {
		t.Fatal("expected error")
	}
}

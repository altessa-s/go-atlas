// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockValidator struct {
	claims *Claims
	err    error
}

func (m *mockValidator) ValidateToken(_ context.Context, _ string) (*Claims, error) {
	return m.claims, m.err
}

func TestAuthFunc_Success(t *testing.T) {
	v := &mockValidator{claims: &Claims{Subject: "user1"}}
	fn := AuthFunc(v)

	req := auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "valid-token"},
	}

	result, err := fn(t.Context(), req)
	require.NoError(t, err)
	claims, ok := result.(*Claims)
	require.True(t, ok, "expected *Claims")
	require.Equal(t, "user1", claims.Subject)
}

func TestAuthFunc_MissingToken(t *testing.T) {
	v := &mockValidator{}
	fn := AuthFunc(v)

	req := auth.Request{Base: auth.Base{AuthMethod: "other"}}

	_, err := fn(t.Context(), req)
	require.NotNil(t, err, "expected error")
	st, ok := status.FromError(err)
	require.True(t, ok, "expected Unauthenticated, got %v", err)
	require.Equal(t, codes.Unauthenticated, st.Code())
}

func TestAuthFunc_ValidationError(t *testing.T) {
	v := &mockValidator{err: status.Error(codes.Unauthenticated, "bad token")}
	fn := AuthFunc(v)

	req := auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "bad"},
	}

	_, err := fn(t.Context(), req)
	require.NotNil(t, err, "expected error")
}

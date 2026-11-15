// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoidc "github.com/altessa-s/go-atlas/auth/oidc"
)

type mockValidator struct {
	claims *authoidc.Claims
	err    error
}

func (m *mockValidator) ValidateToken(_ context.Context, _ string) (*authoidc.Claims, error) {
	return m.claims, m.err
}

func TestAuthFunc_Success(t *testing.T) {
	t.Parallel()
	v := &mockValidator{claims: &authoidc.Claims{Subject: "user1"}}
	fn := AuthFunc(v)

	req := auth.Request{
		AuthMethod: auth.MethodToken,
		Payload:    &auth.TokenCredentials{Token: "valid-token"},
	}

	result, err := fn(t.Context(), req)
	require.NoError(t, err)
	claims, ok := result.(*authoidc.Claims)
	require.True(t, ok, "expected *authoidc.Claims")
	require.Equal(t, "user1", claims.Subject)
}

func TestAuthFunc_MissingToken(t *testing.T) {
	t.Parallel()
	v := &mockValidator{}
	fn := AuthFunc(v)

	req := auth.Request{AuthMethod: "other"}

	_, err := fn(t.Context(), req)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

// TestAuthFunc_ValidationErrorMapsToUnauthenticated pins the adapter's job:
// the transport-neutral validator returns plain errors and the gRPC adapter
// maps every one of them to codes.Unauthenticated.
func TestAuthFunc_ValidationErrorMapsToUnauthenticated(t *testing.T) {
	t.Parallel()
	v := &mockValidator{err: authoidc.ErrTokenInvalid}
	fn := AuthFunc(v)

	req := auth.Request{
		AuthMethod: auth.MethodToken,
		Payload:    &auth.TokenCredentials{Token: "bad"},
	}

	_, err := fn(t.Context(), req)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

// TestAuthFunc_BridgesPrincipal pins that the OIDC path exposes the caller
// through principal.FromContext, as every other authenticator does, while
// the credentials keep carrying the *oidc.Claims.
func TestAuthFunc_BridgesPrincipal(t *testing.T) {
	t.Parallel()
	claims := &authoidc.Claims{
		Subject:   "user1",
		Scopes:    []string{"files:read"},
		RawClaims: map[string]any{"sub": "user1", "tenant": "acme", "roles": []any{"admin"}},
	}
	si := auth.ServerInterceptor(
		auth.WithTokenExtractor(auth.TokenExtractorFunc(func(context.Context) (string, error) {
			return "valid-token", nil
		})),
		auth.WithAuthFn(AuthFunc(&mockValidator{claims: claims})),
	)

	var (
		got  principal.Principal
		ok   bool
		data any
	)
	handler := func(ctx context.Context, _ any) (any, error) {
		got, ok = principal.FromContext(ctx)
		creds, _ := auth.CredentialsFromContext(ctx)
		data = creds.Data
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	_, err := si.ServerUnaryInterceptor()(t.Context(), nil, info, handler)
	require.NoError(t, err)
	require.True(t, ok, "principal.FromContext must be set on the OIDC path")
	require.Equal(t, principal.Principal{
		Subject: "user1", Tenant: "acme", Scopes: []string{"files:read"}, Roles: []string{"admin"},
	}, got)
	require.Same(t, claims, data)
}

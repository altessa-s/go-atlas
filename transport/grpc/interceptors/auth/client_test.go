// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

func TestStaticTokenProvider(t *testing.T) {
	p := StaticTokenProvider("my-token")
	tok, err := p.Token(t.Context())
	require.NoError(t, err)
	require.Equal(t, "my-token", tok)
}

func TestTokenProviderFunc(t *testing.T) {
	f := TokenProviderFunc(func(ctx context.Context) (string, error) {
		return "dynamic", nil
	})
	tok, err := f.Token(t.Context())
	require.NoError(t, err)
	require.Equal(t, "dynamic", tok)
}

func TestPerRPCCredentials_RequireTransportSecurity(t *testing.T) {
	t.Parallel()
	creds := mustCredentials(t, StaticTokenProvider("t"))
	require.True(t, creds.RequireTransportSecurity())
}

func TestPerRPCCredentials_GetRequestMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		token string
		opts  []ClientOption
		want  map[string]string
	}{
		{"bearer by default", "tok123", nil, map[string]string{"authorization": "Bearer tok123"}},
		{"empty token", "", nil, nil},
		{"custom header without scheme", "key", []ClientOption{WithHeaderName("X-Api-Key"), WithNoScheme()}, map[string]string{"x-api-key": "key"}},
		{"custom scheme", "tok", []ClientOption{WithScheme("Token")}, map[string]string{"authorization": "Token tok"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			creds := mustCredentials(t, StaticTokenProvider(tc.token), tc.opts...)
			got, err := creds.GetRequestMetadata(t.Context())
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func mustCredentials(t *testing.T, p TokenProvider, opts ...ClientOption) credentials.PerRPCCredentials {
	t.Helper()
	creds, err := NewPerRPCCredentials(p, opts...)
	require.NoError(t, err)
	return creds
}

func TestNewPerRPCCredentials_RejectsMissingProvider(t *testing.T) {
	t.Parallel()
	var typedNil TokenProviderFunc
	for _, p := range []TokenProvider{nil, typedNil} {
		_, err := NewPerRPCCredentials(p)
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	}
}

func TestPerRPCCredentials_ProviderErrorIsSanitized(t *testing.T) {
	t.Parallel()
	const marker = "SECRET-MARKER-7f3a"
	errToken := errors.New("vault said " + marker)
	creds := mustCredentials(t, TokenProviderFunc(func(context.Context) (string, error) { return "", errToken }))
	_, err := creds.GetRequestMetadata(t.Context())
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.NotContains(t, err.Error(), marker)
}

func TestPerRPCCredentials_RejectsConflictingHeader(t *testing.T) {
	t.Parallel()
	creds := mustCredentials(t, StaticTokenProvider("tok"))
	ctx := metadata.AppendToOutgoingContext(t.Context(), "Authorization", "Bearer other")
	md, err := creds.GetRequestMetadata(ctx)
	require.Nil(t, md)
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	other := metadata.AppendToOutgoingContext(t.Context(), "x-trace", "1")
	got, err := creds.GetRequestMetadata(other)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"authorization": "Bearer tok"}, got)
}

func TestWithHeaderName_Normalizes(t *testing.T) {
	t.Parallel()
	creds := mustCredentials(t, StaticTokenProvider("k"), WithHeaderName("  X-Api-Key "), WithNoScheme())
	got, err := creds.GetRequestMetadata(t.Context())
	require.NoError(t, err)
	require.Equal(t, map[string]string{"x-api-key": "k"}, got)
}

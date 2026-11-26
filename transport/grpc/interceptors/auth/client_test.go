// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
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
	creds := NewPerRPCCredentials(StaticTokenProvider("t"))
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
		{"custom header without scheme", "key", []ClientOption{WithHeaderName("X-Api-Key"), WithScheme("")}, map[string]string{"x-api-key": "key"}},
		{"custom scheme", "tok", []ClientOption{WithScheme("Token")}, map[string]string{"authorization": "Token tok"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			creds := NewPerRPCCredentials(StaticTokenProvider(tc.token), tc.opts...)
			got, err := creds.GetRequestMetadata(t.Context())
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestPerRPCCredentials_ProviderError(t *testing.T) {
	t.Parallel()
	errToken := errors.New("token source down")
	creds := NewPerRPCCredentials(TokenProviderFunc(func(context.Context) (string, error) { return "", errToken }))
	_, err := creds.GetRequestMetadata(t.Context())
	require.ErrorIs(t, err, errToken)
}

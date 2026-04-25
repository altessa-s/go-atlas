// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsvault_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	tlsvault "github.com/altessa-s/go-atlas/security/tlsutils/providers/vault"
)

func TestNew_NoOptions(t *testing.T) {
	// New with no options creates a provider with zero-value defaults
	v, err := tlsvault.New()
	require.NoError(t, err)
	require.NotNil(t, v)
	defer v.Close(t.Context())
}

func TestNew_WithEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"valid https", "https://vault.example.com:8200", false},
		{"valid http", "http://localhost:8200", false},
		{"invalid url", "://invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tlsvault.New(
				tlsvault.WithEndpoint(tt.endpoint),
				tlsvault.WithStaticToken("s.token"),
				tlsvault.WithRole("test-role"),
				tlsvault.WithCommonName("test.example.com"),
			)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestVault_Type(t *testing.T) {
	v, err := tlsvault.New(
		tlsvault.WithEndpoint("https://vault.example.com"),
		tlsvault.WithStaticToken("s.token"),
		tlsvault.WithRole("test-role"),
		tlsvault.WithCommonName("test.example.com"),
	)
	require.NoError(t, err)
	defer v.Close(t.Context())

	require.Equal(t, tlsproviders.ProviderTypeVault, v.Type())
}

func TestVault_TLSConfig(t *testing.T) {
	v, err := tlsvault.New(
		tlsvault.WithEndpoint("https://vault.example.com"),
		tlsvault.WithStaticToken("s.token"),
		tlsvault.WithRole("test-role"),
		tlsvault.WithCommonName("test.example.com"),
	)
	require.NoError(t, err)
	defer v.Close(t.Context())

	cfg, err := v.TLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.GetCertificate)
	require.NotNil(t, cfg.GetClientCertificate)
}

func TestVault_Close(t *testing.T) {
	v, err := tlsvault.New(
		tlsvault.WithEndpoint("https://vault.example.com"),
		tlsvault.WithStaticToken("s.token"),
		tlsvault.WithRole("test-role"),
		tlsvault.WithCommonName("test.example.com"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	require.NoError(t, v.Close(ctx))
}

func TestRSAGenerator_Generate(t *testing.T) {
	gen := tlsvault.NewRSAGenerator()
	key, err := gen.Generate()
	require.NoError(t, err)
	require.NotNil(t, key)

	// Second call should return same key
	key2, err := gen.Generate()
	require.NoError(t, err)
	require.Equal(t, key, key2)
}

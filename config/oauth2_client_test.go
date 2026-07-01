// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

func TestOAuth2ClientValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     config.OAuth2Client
		wantErr bool
	}{
		{
			name: "valid with token url",
			cfg:  config.OAuth2Client{TokenUrl: "https://idp.example/token", ClientId: "svc", AuthStyle: "auto"},
		},
		{
			name: "valid with discovery url",
			cfg:  config.OAuth2Client{DiscoveryUrl: "https://idp.example/.well-known/openid-configuration", ClientId: "svc", AuthStyle: "header"},
		},
		{
			name:    "missing both endpoints",
			cfg:     config.OAuth2Client{ClientId: "svc", AuthStyle: "auto"},
			wantErr: true,
		},
		{
			name:    "missing client id",
			cfg:     config.OAuth2Client{TokenUrl: "https://idp.example/token", AuthStyle: "auto"},
			wantErr: true,
		},
		{
			name:    "bad token url",
			cfg:     config.OAuth2Client{TokenUrl: "not-a-url", ClientId: "svc", AuthStyle: "auto"},
			wantErr: true,
		},
		{
			name:    "bad auth style",
			cfg:     config.OAuth2Client{TokenUrl: "https://idp.example/token", ClientId: "svc", AuthStyle: "basic"},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestOAuth2ClientAuthValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		auth    config.OAuth2ClientAuth
		wantErr bool
	}{
		{name: "client_secret_jwt", auth: config.OAuth2ClientAuth{Method: "client_secret_jwt"}},
		{
			name: "private_key_jwt valid",
			auth: config.OAuth2ClientAuth{Method: "private_key_jwt", PrivateKey: config.Secret("pem"), KeyId: "k", Algorithm: "RS256"},
		},
		{name: "unknown method", auth: config.OAuth2ClientAuth{Method: "basic"}, wantErr: true},
		{
			name:    "private_key_jwt missing key",
			auth:    config.OAuth2ClientAuth{Method: "private_key_jwt", KeyId: "k", Algorithm: "RS256"},
			wantErr: true,
		},
		{
			name:    "private_key_jwt bad algorithm",
			auth:    config.OAuth2ClientAuth{Method: "private_key_jwt", PrivateKey: config.Secret("pem"), KeyId: "k", Algorithm: "HS256"},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.auth.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestOAuth2ClientHelpers(t *testing.T) {
	t.Parallel()
	disco := config.OAuth2Client{DiscoveryUrl: "https://idp.example/.well-known/openid-configuration", ClientId: "svc"}
	require.True(t, disco.IsDiscovery())
	require.False(t, disco.IsRetryConfigured())

	direct := config.OAuth2Client{TokenUrl: "https://idp.example/token", ClientId: "svc", Retry: &config.OAuth2ClientRetry{Attempts: 3}}
	require.False(t, direct.IsDiscovery())
	require.True(t, direct.IsRetryConfigured())
}

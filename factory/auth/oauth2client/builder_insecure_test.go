// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
	factory "github.com/altessa-s/go-atlas/factory/auth/oauth2client"
)

// The builder rejects a plain-http token endpoint, configured or discovered,
// unless the config opts in with allowInsecureHTTP.
func TestBuilderRejectsInsecureTokenEndpoint(t *testing.T) {
	t.Parallel()
	const insecureURL = "http://idp.example/token"

	tests := []struct {
		name    string
		cfg     authconfig.OAuth2Client
		source  oauth2client.TokenEndpointSource
		wantErr bool
	}{
		{"configured http endpoint", authconfig.OAuth2Client{TokenUrl: insecureURL}, nil, true},
		{
			"discovered http endpoint",
			authconfig.OAuth2Client{DiscoveryUrl: "https://idp.example/.well-known/openid-configuration"},
			staticEndpoint(insecureURL), true,
		},
		{"configured http endpoint with opt-in", authconfig.OAuth2Client{TokenUrl: insecureURL, AllowInsecureHTTP: true}, nil, false},
		{
			"discovered http endpoint with opt-in",
			authconfig.OAuth2Client{DiscoveryUrl: "https://idp.example/.well-known/openid-configuration", AllowInsecureHTTP: true},
			staticEndpoint(insecureURL), false,
		},
		{
			"discovered https endpoint on another host",
			authconfig.OAuth2Client{DiscoveryUrl: "https://idp.example/.well-known/openid-configuration"},
			staticEndpoint("https://tokens.idp.example/token"), false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := tc.cfg
			cfg.ClientId = "svc"
			cfg.AuthStyle = "auto"
			b := factory.New(&cfg)
			if tc.source != nil {
				b.UseTokenEndpointSource(tc.source)
			}

			_, buildErr := b.Build(t.Context())
			_, exchangerErr := b.BuildExchanger(t.Context())

			if tc.wantErr {
				require.ErrorIs(t, buildErr, oauth2client.ErrInsecureEndpoint)
				require.ErrorIs(t, exchangerErr, oauth2client.ErrInsecureEndpoint)
				return
			}
			require.NoError(t, buildErr)
			require.NoError(t, exchangerErr)
		})
	}
}

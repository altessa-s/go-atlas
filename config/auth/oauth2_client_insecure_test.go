// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

// The client credentials are sent to the token endpoint, so it must be https
// unless it is a loopback host or the config opts in.
func TestOAuth2ClientValidate_EndpointScheme(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     authconfig.OAuth2Client
		wantErr bool
	}{
		{"https token url", authconfig.OAuth2Client{TokenUrl: "https://idp.example/token"}, false},
		{"http token url", authconfig.OAuth2Client{TokenUrl: "http://idp.example/token"}, true},
		{"http discovery url", authconfig.OAuth2Client{DiscoveryUrl: "http://idp.example/.well-known/openid-configuration"}, true},
		{"http token url on localhost", authconfig.OAuth2Client{TokenUrl: "http://localhost:8080/token"}, false},
		{"http token url on loopback IP", authconfig.OAuth2Client{TokenUrl: "http://127.0.0.1:8080/token"}, false},
		{"http discovery url on loopback IP", authconfig.OAuth2Client{DiscoveryUrl: "http://127.0.0.1:8080/.well-known/openid-configuration"}, false},
		{"http token url on a host that only starts with localhost", authconfig.OAuth2Client{TokenUrl: "http://localhost.evil.example/token"}, true},
		{"http token url with opt-in", authconfig.OAuth2Client{TokenUrl: "http://idp.example/token", AllowInsecureHTTP: true}, false},
		{
			"http discovery url with opt-in",
			authconfig.OAuth2Client{DiscoveryUrl: "http://idp.example/.well-known/openid-configuration", AllowInsecureHTTP: true}, false,
		},
		{"other scheme is not unlocked by the opt-in", authconfig.OAuth2Client{TokenUrl: "ftp://idp.example/token", AllowInsecureHTTP: true}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := tc.cfg
			cfg.ClientId = "svc"
			cfg.AuthStyle = "auto"
			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

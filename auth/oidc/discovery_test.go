// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDiscoveryIsValidTokenOnly verifies a token-only IdP — one that omits the
// authorization, token, and userinfo endpoints (userinfo_endpoint is only
// RECOMMENDED by OpenID Connect Discovery 1.0 §3) — passes validation, while a
// document missing a field token verification actually needs does not.
func TestDiscoveryIsValidTokenOnly(t *testing.T) {
	t.Parallel()

	valid := &discoveryInfo{
		Issuer:     "https://issuer",
		JwksURL:    "https://issuer/jwks",
		Algorithms: []string{"RS256"},
	}
	require.True(t, valid.IsValid(), "token-only discovery (no auth/token/userinfo endpoints) must be valid")

	require.False(t, (&discoveryInfo{JwksURL: "https://issuer/jwks", Algorithms: []string{"RS256"}}).IsValid(), "missing issuer")
	require.False(t, (&discoveryInfo{Issuer: "https://issuer", Algorithms: []string{"RS256"}}).IsValid(), "missing jwks_uri")
	require.False(t, (&discoveryInfo{Issuer: "https://issuer", JwksURL: "https://issuer/jwks"}).IsValid(), "missing algorithms")
}

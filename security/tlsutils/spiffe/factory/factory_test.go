// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/security/tlsutils/spiffe/factory"
)

func TestAuthorizerTrustDomain(t *testing.T) {
	t.Parallel()
	auth, err := factory.New(&config.SPIFFE{AllowedTrustDomains: []string{"example.org", "other.org"}}).Authorizer()
	require.NoError(t, err)

	require.NoError(t, auth(spiffeid.RequireFromString("spiffe://example.org/sa/billing"), nil))
	require.NoError(t, auth(spiffeid.RequireFromString("spiffe://other.org/sa/x"), nil))
	require.Error(t, auth(spiffeid.RequireFromString("spiffe://evil.org/sa/x"), nil))
}

func TestAuthorizerIDs(t *testing.T) {
	t.Parallel()
	auth, err := factory.New(&config.SPIFFE{AllowedIDs: []string{"spiffe://example.org/sa/billing"}}).Authorizer()
	require.NoError(t, err)

	require.NoError(t, auth(spiffeid.RequireFromString("spiffe://example.org/sa/billing"), nil))
	// Same trust domain, different path is rejected — IDs match exactly.
	require.Error(t, auth(spiffeid.RequireFromString("spiffe://example.org/sa/other"), nil))
}

func TestAuthorizerRequiresSource(t *testing.T) {
	t.Parallel()
	_, err := factory.New(&config.SPIFFE{}).Authorizer()
	require.ErrorIs(t, err, factory.ErrNoAuthorizerSource)
}

func TestAuthorizerNilConfig(t *testing.T) {
	t.Parallel()
	_, err := factory.New(nil).Authorizer()
	require.ErrorIs(t, err, factory.ErrNoConfig)
}

func TestAuthorizerInvalidInput(t *testing.T) {
	t.Parallel()
	_, err := factory.New(&config.SPIFFE{AllowedIDs: []string{"not-a-spiffe-id"}}).Authorizer()
	require.Error(t, err)

	_, err = factory.New(&config.SPIFFE{AllowedTrustDomains: []string{"bad domain"}}).Authorizer()
	require.Error(t, err)
}

func TestOptions(t *testing.T) {
	t.Parallel()
	opts, err := factory.New(&config.SPIFFE{
		AllowedTrustDomains: []string{"example.org"},
		SocketPath:          "unix:///run/spire/agent/api.sock",
	}).Options()
	require.NoError(t, err)
	require.Len(t, opts, 2) // authorizer + socket path

	opts, err = factory.New(&config.SPIFFE{AllowedTrustDomains: []string{"example.org"}}).Options()
	require.NoError(t, err)
	require.Len(t, opts, 1) // authorizer only
}

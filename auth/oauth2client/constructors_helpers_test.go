// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"

	"golang.org/x/oauth2"
)

// The must* helpers build a flow whose endpoint URLs the test knows to be
// acceptable, failing the test when the constructor rejects them.

func mustClientCredentials(t testing.TB, tokenURL, clientID, clientSecret string, opts ...oauth2client.ClientCredentialsOption) oauth2.TokenSource {
	t.Helper()
	src, err := oauth2client.ClientCredentials(t.Context(), tokenURL, clientID, clientSecret, opts...)
	require.NoError(t, err)
	return src
}

func mustRefresh(t testing.TB, tokenURL, clientID, clientSecret, refreshToken string, opts ...oauth2client.RefreshOption) oauth2.TokenSource {
	t.Helper()
	src, err := oauth2client.Refresh(t.Context(), tokenURL, clientID, clientSecret, refreshToken, opts...)
	require.NoError(t, err)
	return src
}

func mustExchanger(t testing.TB, tokenURL, clientID, clientSecret string, opts ...oauth2client.ExchangerOption) *oauth2client.Exchanger {
	t.Helper()
	ex, err := oauth2client.NewExchanger(tokenURL, clientID, clientSecret, opts...)
	require.NoError(t, err)
	return ex
}

func mustRevoker(t testing.TB, revocationURL, clientID, clientSecret string, opts ...oauth2client.RevokerOption) *oauth2client.Revoker {
	t.Helper()
	r, err := oauth2client.NewRevoker(revocationURL, clientID, clientSecret, opts...)
	require.NoError(t, err)
	return r
}

func mustDeviceFlow(t testing.TB, endpoint oauth2.Endpoint, clientID, clientSecret string, opts ...oauth2client.DeviceFlowOption) *oauth2client.DeviceFlow {
	t.Helper()
	df, err := oauth2client.NewDeviceFlow(endpoint, clientID, clientSecret, opts...)
	require.NoError(t, err)
	return df
}

// allowInsecureHTTP supplies the grant's opt-in only for the plain-HTTP test cases.
func allowInsecureHTTP[O any](allow bool, enable func() O, opts ...O) []O {
	if allow {
		return append(opts, enable())
	}
	return opts
}

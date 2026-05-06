// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxydial

import (
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSConfig_NilUserAppliesSafeDefaults(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://proxy.example.com:8443")
	require.NoError(t, err)

	got := TLSConfig(u, nil)
	require.NotNil(t, got)
	require.Equal(t, "proxy.example.com", got.ServerName)
	require.Equal(t, uint16(tls.VersionTLS12), got.MinVersion)
}

func TestTLSConfig_FillsMissingDefaults(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://proxy.example.com:8443")
	require.NoError(t, err)

	pool := x509.NewCertPool()
	user := &tls.Config{RootCAs: pool} // ServerName="" and MinVersion=0 → defaults filled

	got := TLSConfig(u, user)
	require.NotSame(t, user, got, "user config must be cloned, not mutated")
	require.Equal(t, "proxy.example.com", got.ServerName)
	require.Equal(t, uint16(tls.VersionTLS12), got.MinVersion)
	require.Same(t, pool, got.RootCAs, "non-default fields must be preserved verbatim")

	// Original user value must remain untouched.
	require.Empty(t, user.ServerName)
	require.Zero(t, user.MinVersion)
}

func TestTLSConfig_PreservesUserOverrides(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://proxy.example.com:8443")
	require.NoError(t, err)

	user := &tls.Config{ServerName: "alt.example", MinVersion: tls.VersionTLS13}
	got := TLSConfig(u, user)
	require.Equal(t, "alt.example", got.ServerName)
	require.Equal(t, uint16(tls.VersionTLS13), got.MinVersion)
}

func TestBasicAuthHeader(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		auth *url.Userinfo
		want string
	}{
		{"user_password", url.UserPassword("svc", "secret"), "Basic c3ZjOnNlY3JldA=="},
		{"username_only", url.User("svc"), "Basic c3ZjOg=="},
		{"empty", url.UserPassword("", ""), "Basic Og=="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, basicAuthHeader(tc.auth))
		})
	}
}

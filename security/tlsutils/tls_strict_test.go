// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsutils_test

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/tlsutils"
)

func TestDefaultTLSConfigStrict_RequiresTLS13(t *testing.T) {
	t.Parallel()

	cfg := tlsutils.DefaultTLSConfigStrict()
	require.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion,
		"strict config must pin MinVersion to TLS 1.3")
	require.Empty(t, cfg.CipherSuites,
		"strict config must NOT pin a TLS 1.2 cipher list — stdlib's TLS 1.3 AEAD set governs")
}

func TestDefaultClientTLSConfigStrict_PinsServerName(t *testing.T) {
	t.Parallel()

	cfg := tlsutils.DefaultClientTLSConfigStrict("example.com")
	require.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	require.Equal(t, "example.com", cfg.ServerName)
}

// TestResolveMinTLSVersion_KnownValues pins the enum semantics that
// operators rely on when reading values from YAML.
func TestResolveMinTLSVersion_KnownValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want uint16
	}{
		{"1.2", tls.VersionTLS12},
		{"1.3", tls.VersionTLS13},
		{"TLS1.3", tls.VersionTLS13},
		{"tlsv1.2", tls.VersionTLS12},
		{"  1.3  ", tls.VersionTLS13},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := tlsutils.ResolveMinTLSVersion(tc.in)
			require.True(t, ok, "%q must be recognized", tc.in)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestResolveMinTLSVersion_UnknownFailsSafe is the regression guard
// for the typo case: an unrecognized version must report ok=false so
// the caller surfaces a config error rather than silently accepting
// the value and falling through to whatever the stdlib defaults to.
func TestResolveMinTLSVersion_UnknownFailsSafe(t *testing.T) {
	t.Parallel()

	cases := []string{"", "1.1", "ssl3", "1.4", "TLS"}
	for _, in := range cases {
		got, ok := tlsutils.ResolveMinTLSVersion(in)
		require.False(t, ok, "%q must NOT be recognized — caller should treat this as a config error", in)
		require.Equal(t, uint16(tls.VersionTLS12), got,
			"fallback value must be the safe default (TLS 1.2), never zero")
	}
}

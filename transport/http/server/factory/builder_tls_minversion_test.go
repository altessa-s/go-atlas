// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestApplyMinTLSVersion_Empty preserves the provider-supplied TLS
// config when the operator omits minVersion. This is the legacy path
// — callers that never set minVersion must keep getting the provider
// default (TLS 1.2 + pinned 1.2 cipher list).
func TestApplyMinTLSVersion_Empty(t *testing.T) {
	t.Parallel()

	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
	}
	require.NoError(t, applyMinTLSVersion(cfg, ""))
	require.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion, "empty minVersion must not change MinVersion")
	require.NotEmpty(t, cfg.CipherSuites, "empty minVersion must not touch CipherSuites")
}

// TestApplyMinTLSVersion_TLS13_ClearsCipherSuites is the strict-mode
// regression: pinning TLS 1.3 via YAML must clear the 1.2-only cipher
// list seeded by the provider, matching DefaultTLSConfigStrict
// semantics. Without the clear, operators reading the resulting
// tls.Config would see a CipherSuites field that the stdlib silently
// ignores under TLS 1.3 — misleading documentation of intent.
func TestApplyMinTLSVersion_TLS13_ClearsCipherSuites(t *testing.T) {
	t.Parallel()

	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		},
	}
	require.NoError(t, applyMinTLSVersion(cfg, "1.3"))
	require.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	require.Nil(t, cfg.CipherSuites, "TLS 1.3 must clear CipherSuites to match DefaultTLSConfigStrict")
}

// TestApplyMinTLSVersion_TLS12_PreservesCipherSuites confirms the
// strict-mode clear is gated on 1.3 — pinning 1.2 explicitly must
// leave the provider's 1.2 cipher list intact.
func TestApplyMinTLSVersion_TLS12_PreservesCipherSuites(t *testing.T) {
	t.Parallel()

	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
	}
	require.NoError(t, applyMinTLSVersion(cfg, "1.2"))
	require.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	require.Len(t, cfg.CipherSuites, 1, "TLS 1.2 must NOT clear CipherSuites")
}

// TestApplyMinTLSVersion_Unknown rejects typos at config-build time
// instead of silently falling through to the stdlib default — the
// safety guarantee the ResolveMinTLSVersion fix introduced. Before
// the migration, "v1.3" matched neither branch of the == comparison
// and the server quietly came up with the provider default.
func TestApplyMinTLSVersion_Unknown(t *testing.T) {
	t.Parallel()

	cases := []string{"v1.3", "tls13", "TLS-1.3", "1.4", "garbage"}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			cfg := &tls.Config{MinVersion: tls.VersionTLS12}
			err := applyMinTLSVersion(cfg, raw)
			require.Error(t, err, "unknown minVersion %q must be rejected", raw)
			require.Contains(t, err.Error(), raw, "error must echo the offending value")
		})
	}
}

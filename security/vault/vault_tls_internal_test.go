// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	vaultApi "github.com/hashicorp/vault/api"
)

type stubRoundTripper struct{}

func (stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) { return nil, http.ErrSkipAltProtocol }

// White-box: vaultApi.DefaultConfig always yields an *http.Transport, so the
// rejected-transport branch is reachable only through applyTLSConfig.
func TestApplyTLSConfig(t *testing.T) {
	t.Parallel()
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}

	t.Run("foreign transport is rejected", func(t *testing.T) {
		t.Parallel()
		cfg := &vaultApi.Config{HttpClient: &http.Client{Transport: stubRoundTripper{}}}
		require.ErrorIs(t, applyTLSConfig(cfg, tlsCfg), ErrTLSConfigNotApplied)
	})

	t.Run("nil config is a no-op for any transport", func(t *testing.T) {
		t.Parallel()
		cfg := &vaultApi.Config{HttpClient: &http.Client{Transport: stubRoundTripper{}}}
		require.NoError(t, applyTLSConfig(cfg, nil))
	})

	t.Run("http transport receives the config", func(t *testing.T) {
		t.Parallel()
		tr := &http.Transport{}
		cfg := &vaultApi.Config{HttpClient: &http.Client{Transport: tr}}
		require.NoError(t, applyTLSConfig(cfg, tlsCfg))
		require.Same(t, tlsCfg, tr.TLSClientConfig)
	})
}

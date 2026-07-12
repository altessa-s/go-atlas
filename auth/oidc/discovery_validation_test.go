// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestDeriveExpectedIssuer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"openid_configuration", "https://issuer.example.com/.well-known/openid-configuration", "https://issuer.example.com"},
		{"oauth_authorization_server", "https://issuer.example.com/.well-known/oauth-authorization-server", "https://issuer.example.com"},
		{"trailing_slash", "https://issuer.example.com/", "https://issuer.example.com"},
		{"path_issuer", "https://host.example.com/tenant/.well-known/openid-configuration", "https://host.example.com/tenant"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, deriveExpectedIssuer(tc.url))
		})
	}
}

// discoveryProvider builds a Provider whose discovery fetch returns body, at
// the given validation mode.
func discoveryProvider(t *testing.T, mode DiscoveryValidationMode, logger *slog.Logger, body string) *Provider {
	t.Helper()
	rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Request:    r,
		}, nil
	})
	return &Provider{
		discoveryURL: "https://issuer.example.com/.well-known/openid-configuration",
		client:       &http.Client{Transport: rt},
		logger:       logger,
		opts:         &options{discoveryValidationMode: mode},
	}
}

const validDiscoveryBody = `{
  "issuer":"https://issuer.example.com",
  "authorization_endpoint":"https://issuer.example.com/auth",
  "token_endpoint":"https://issuer.example.com/token",
  "jwks_uri":"https://issuer.example.com/jwks",
  "userinfo_endpoint":"https://issuer.example.com/userinfo",
  "introspection_endpoint":"https://issuer.example.com/introspect",
  "id_token_signing_alg_values_supported":["RS256"]
}`

func TestGetDiscoveryInfo_Enforce_AcceptsConformingDocument(t *testing.T) {
	t.Parallel()

	p := discoveryProvider(t, DiscoveryValidationModeEnforce, slog.New(slog.DiscardHandler), validDiscoveryBody)
	require.NoError(t, p.getDiscoveryInfo(t.Context()))
	require.NotNil(t, p.discoveryInfo)
}

func TestGetDiscoveryInfo_Enforce_RejectsIssuerMismatch(t *testing.T) {
	t.Parallel()

	body := `{
  "issuer":"https://evil.example.com",
  "jwks_uri":"https://issuer.example.com/jwks",
  "id_token_signing_alg_values_supported":["RS256"]
}`
	p := discoveryProvider(t, DiscoveryValidationModeEnforce, slog.New(slog.DiscardHandler), body)
	err := p.getDiscoveryInfo(t.Context())
	require.ErrorIs(t, err, ErrDiscoveryValidation)
	require.Nil(t, p.discoveryInfo)
}

func TestGetDiscoveryInfo_Enforce_RejectsCrossOriginEndpoint(t *testing.T) {
	t.Parallel()

	// introspection_endpoint points at an attacker host — the client-secret
	// exfiltration vector.
	body := `{
  "issuer":"https://issuer.example.com",
  "jwks_uri":"https://issuer.example.com/jwks",
  "introspection_endpoint":"https://attacker.example.net/introspect",
  "id_token_signing_alg_values_supported":["RS256"]
}`
	p := discoveryProvider(t, DiscoveryValidationModeEnforce, slog.New(slog.DiscardHandler), body)
	err := p.getDiscoveryInfo(t.Context())
	require.ErrorIs(t, err, ErrDiscoveryValidation)
	require.Nil(t, p.discoveryInfo)
}

func TestGetDiscoveryInfo_Enforce_RejectsNonHTTPSEndpoint(t *testing.T) {
	t.Parallel()

	body := `{
  "issuer":"https://issuer.example.com",
  "jwks_uri":"http://issuer.example.com/jwks",
  "id_token_signing_alg_values_supported":["RS256"]
}`
	p := discoveryProvider(t, DiscoveryValidationModeEnforce, slog.New(slog.DiscardHandler), body)
	err := p.getDiscoveryInfo(t.Context())
	require.ErrorIs(t, err, ErrDiscoveryValidation)
	require.Nil(t, p.discoveryInfo)
}

func TestGetDiscoveryInfo_Warn_LoadsButLogs(t *testing.T) {
	t.Parallel()

	body := `{
  "issuer":"https://evil.example.com",
  "jwks_uri":"https://issuer.example.com/jwks",
  "id_token_signing_alg_values_supported":["RS256"]
}`
	rec := &recordingHandler{}
	p := discoveryProvider(t, DiscoveryValidationModeWarn, slog.New(rec), body)
	require.NoError(t, p.getDiscoveryInfo(t.Context()))
	require.NotNil(t, p.discoveryInfo, "warn mode loads the document")
	require.GreaterOrEqual(t, rec.errorCount, 1, "warn mode logs the violation at Error level")
}

func TestGetDiscoveryInfo_Disabled_SkipsValidation(t *testing.T) {
	t.Parallel()

	body := `{
  "issuer":"https://evil.example.com",
  "jwks_uri":"https://attacker.example.net/jwks",
  "id_token_signing_alg_values_supported":["RS256"]
}`
	p := discoveryProvider(t, DiscoveryValidationModeDisabled, slog.New(slog.DiscardHandler), body)
	require.NoError(t, p.getDiscoveryInfo(t.Context()))
	require.NotNil(t, p.discoveryInfo, "disabled mode trusts the document as-is")
}

// recordingHandler counts Error-level records.
type recordingHandler struct{ errorCount int }

func (h *recordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		h.errorCount++
	}
	return nil
}
func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler      { return h }

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

type discoveryClient struct {
	discoveryURL               string
	client                     *http.Client
	info                       *discoveryInfo
	logger                     *slog.Logger
	discoveryValidationMode    DiscoveryValidationMode
	discoveryAllowLoopbackHTTP bool
}

type discoveryInfo struct {
	Issuer           string   `json:"issuer"`
	AuthURL          string   `json:"authorization_endpoint"`
	TokenURL         string   `json:"token_endpoint"`
	JwksURL          string   `json:"jwks_uri"`
	UserInfoURL      string   `json:"userinfo_endpoint"`
	IntrospectionURL string   `json:"introspection_endpoint"`
	EndSessionURL    string   `json:"end_session_endpoint"`
	RevocationURL    string   `json:"revocation_endpoint"`
	Algorithms       []string `json:"id_token_signing_alg_values_supported"`
}

// IsValid reports whether the discovery document carries the fields this
// provider needs to verify tokens: the issuer, the JWKS URI, and at least one
// supported signing algorithm. The authorization, token, and userinfo endpoints
// are intentionally not required — they belong to the OAuth flow / UserInfo, not
// to token validation, and per OpenID Connect Discovery 1.0 §3 userinfo_endpoint
// is only RECOMMENDED. Requiring them would reject spec-valid token-only IdPs.
// UserInfo calls guard the empty endpoint themselves.
func (d *discoveryInfo) IsValid() bool {
	return d.Issuer != "" &&
		d.JwksURL != "" &&
		len(d.Algorithms) > 0
}

func (p *discoveryClient) getDiscoveryInfo(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.discoveryURL, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req) //nolint:bodyclose
	if err != nil {
		return coreerrs.WrapOperation(errors.Join(ErrDiscovery, err), "fetch discovery")
	}
	defer drainAndClose(resp)

	// Security: cap the read — a malicious or misconfigured IdP must not be
	// able to OOM the validator with a multi-GB discovery document.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscoveryResponseSize+1))
	if err != nil {
		return coreerrs.Wrapf(ErrDiscovery, "%s", err)
	}
	if len(body) > maxDiscoveryResponseSize {
		return coreerrs.Wrapf(ErrDiscovery, "discovery document exceeds %d bytes", maxDiscoveryResponseSize)
	}

	if resp.StatusCode != http.StatusOK {
		return coreerrs.Wrapf(ErrDiscovery, "unexpected response status: %s", resp.Status)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "application/json") {
		return coreerrs.Wrapf(ErrDiscovery, "unexpected content type: %s", ct)
	}

	var dInfo discoveryInfo
	if err = json.Unmarshal(body, &dInfo); err != nil {
		return coreerrs.Wrapf(ErrDiscovery, "failed to unmarshal response body: %v", err)
	}

	if !dInfo.IsValid() {
		return coreerrs.Wrap(ErrDiscovery, "received uncompleted discovery info")
	}

	// Mix-up / SSRF defense: verify the document's issuer and the endpoints it
	// advertises before any of them are fetched or sent credentials. A
	// malicious or compromised IdP (or a network attacker on a non-HTTPS
	// fetch) must not be able to redirect JWKS/introspection/userinfo calls to
	// an attacker-chosen host.
	if err = p.validateDiscoveredEndpoints(&dInfo); err != nil {
		return err
	}

	p.info = &dInfo

	return nil
}

// deriveExpectedIssuer strips the well-known discovery suffix from the
// configured discovery URL to recover the issuer identifier that the discovery
// document's issuer field must equal (RFC 8414 §3 / OIDC Discovery §4).
// Trailing slashes are trimmed so the comparison is stable.
func deriveExpectedIssuer(discoveryURL string) string {
	issuer := discoveryURL
	for _, suffix := range []string{
		"/.well-known/openid-configuration",
		"/.well-known/oauth-authorization-server",
	} {
		if trimmed, ok := strings.CutSuffix(issuer, suffix); ok {
			issuer = trimmed
			break
		}
	}
	return strings.TrimRight(issuer, "/")
}

// validateDiscoveredEndpoints enforces the discovery issuer/endpoint contract
// selected by [WithDiscoveryValidationMode]: the document's issuer must equal
// the configured issuer, and every advertised endpoint must be HTTPS and share
// the issuer's host — plain http is accepted on a loopback issuer host with
// [WithDiscoveryAllowLoopbackHTTP]. In [DiscoveryValidationModeWarn] violations are logged and
// the document is accepted; in [DiscoveryValidationModeDisabled] the check is
// skipped entirely.
func (p *discoveryClient) validateDiscoveredEndpoints(d *discoveryInfo) error {
	if p.discoveryValidationMode == DiscoveryValidationModeDisabled {
		return nil
	}

	expectedIssuer := deriveExpectedIssuer(p.discoveryURL)

	var violations []string
	if strings.TrimRight(d.Issuer, "/") != expectedIssuer {
		violations = append(violations,
			fmt.Sprintf("issuer %q does not match configured issuer %q", d.Issuer, expectedIssuer))
	}

	issuerHost := ""
	allowHTTP := false
	if u, err := url.Parse(expectedIssuer); err == nil {
		issuerHost = u.Host
		allowHTTP = p.discoveryAllowLoopbackHTTP && isLoopbackHost(u.Hostname())
		if u.Scheme != "https" && (u.Scheme != "http" || !allowHTTP) {
			violations = append(violations, fmt.Sprintf("issuer %q is not HTTPS", expectedIssuer))
		}
	}

	for _, ep := range []struct{ name, raw string }{
		{"jwks_uri", d.JwksURL},
		{"token_endpoint", d.TokenURL},
		{"introspection_endpoint", d.IntrospectionURL},
		{"userinfo_endpoint", d.UserInfoURL},
		{"authorization_endpoint", d.AuthURL},
		{"end_session_endpoint", d.EndSessionURL},
		{"revocation_endpoint", d.RevocationURL},
	} {
		if ep.raw == "" {
			continue
		}
		u, err := url.Parse(ep.raw)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s %q is not a valid URL", ep.name, ep.raw))
			continue
		}
		// allowHTTP holds only for a loopback issuer, and the host check
		// below pins the endpoint to that same host.
		if u.Scheme != "https" && (u.Scheme != "http" || !allowHTTP) {
			violations = append(violations, fmt.Sprintf("%s %q is not HTTPS", ep.name, ep.raw))
		}
		if issuerHost != "" && u.Host != issuerHost {
			violations = append(violations,
				fmt.Sprintf("%s host %q does not match issuer host %q", ep.name, u.Host, issuerHost))
		}
	}

	if len(violations) == 0 {
		return nil
	}

	if p.discoveryValidationMode == DiscoveryValidationModeWarn {
		p.logger.Error("discovery document failed issuer/endpoint validation; loading anyway (warn mode)",
			slog.String("discovery_url", p.discoveryURL),
			slog.Any("violations", violations))
		return nil
	}

	return coreerrs.Wrapf(ErrDiscoveryValidation, "%s", strings.Join(violations, "; "))
}

// isLoopbackHost reports whether host (without port) is "localhost" or a
// loopback IP literal.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

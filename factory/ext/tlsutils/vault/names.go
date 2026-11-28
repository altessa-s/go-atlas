// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas-core/retry"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	tlsfactory "github.com/altessa-s/go-atlas/factory/ext/tlsutils"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
)

var (
	_ tlsproviders.Provider          = (*configuredNames)(nil)
	_ tlsproviders.ClientCertificate = (*configuredNames)(nil)
	_ tlsfactory.Warmer              = (*configuredNames)(nil)
)

// issueRetryDelay is the first pause between attempts to obtain a certificate while the provider warms up.
const issueRetryDelay = time.Second

// ErrNoClientCertificate is returned by GetClientCertificate when the Vault provider issues no client certificate.
var ErrNoClientCertificate = errors.New("the vault TLS provider issues no client certificate")

// configuredNames wraps the Vault provider. With fallback set, a ClientHello without a server name (a probe, or any
// client dialing a Pod IP) or with a name the role cannot issue gets the certificate of the common name, so no
// handshake asks Vault for a name it refuses. With issueOnBuild set, Warm obtains the certificates of the names.
type configuredNames struct {
	tlsproviders.Provider

	cfg          *tlsconfig.ProviderVault
	names        []string
	fallback     bool
	issueOnBuild bool
}

func newConfiguredNames(p tlsproviders.Provider, cfg *tlsconfig.ProviderVault, fallback, issueOnBuild bool) *configuredNames {
	names := coreslices.To(append([]string{cfg.CommonName}, cfg.SubjectAlternativeNames...), strings.ToLower)
	slices.Sort(names)

	return &configuredNames{Provider: p, cfg: cfg, names: slices.Compact(names), fallback: fallback, issueOnBuild: issueOnBuild}
}

// TLSConfig returns the server configuration of the provider with the common name standing in for a missing or
// unknown server name.
func (p *configuredNames) TLSConfig() (*tls.Config, error) {
	cfg, err := p.Provider.TLSConfig()
	if err != nil || !p.fallback {
		return cfg, err
	}

	certificate := cfg.GetCertificate
	cfg.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if !slices.Contains(p.names, strings.TrimSuffix(strings.ToLower(hello.ServerName), ".")) {
			named := *hello
			named.ServerName = p.cfg.CommonName
			hello = &named
		}

		return certificate(hello)
	}

	return cfg, nil
}

// GetClientCertificate presents the certificate of the common name to a server that asks for a client certificate.
func (p *configuredNames) GetClientCertificate(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	source, ok := p.Provider.(tlsproviders.ClientCertificate)
	if !ok {
		return nil, ErrNoClientCertificate
	}

	return source.GetClientCertificate(info)
}

// Warm obtains the certificate of every configured name when the builder asked for it with [WithIssueOnBuild]. It
// retries until ctx ends and fails when Vault issues none, or issues one that does not outlive renewBefore: such a
// role TTL would renew on every handshake.
func (p *configuredNames) Warm(ctx context.Context) error {
	if !p.issueOnBuild {
		return nil
	}

	cfg, err := p.TLSConfig()
	if err != nil {
		return fmt.Errorf("build the Vault TLS configuration: %w", err)
	}

	for _, name := range p.names {
		var certificate *tls.Certificate
		err := retry.Do(ctx, func(context.Context) error {
			var issueErr error
			certificate, issueErr = cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: name})

			return issueErr
		}, retry.WithMaxAttempts(-1), retry.WithNextDelay(retry.Exponential(issueRetryDelay)), retry.WithStopBeforeDeadline())
		if err != nil {
			return fmt.Errorf("obtain the TLS certificate of %s from Vault role %s: %w", name, p.cfg.Role, err)
		}

		leaf := certificate.Leaf
		if leaf == nil && len(certificate.Certificate) > 0 {
			if leaf, err = x509.ParseCertificate(certificate.Certificate[0]); err != nil {
				return fmt.Errorf("parse the TLS certificate of %s from Vault role %s: %w", name, p.cfg.Role, err)
			}
		}
		if leaf == nil {
			return fmt.Errorf("no TLS certificate of %s from Vault role %s", name, p.cfg.Role)
		}
		if valid := time.Until(leaf.NotAfter); valid <= p.cfg.RenewBefore {
			return fmt.Errorf("the certificate of %s from Vault role %s is valid for %s, not longer than renewBefore %s",
				name, p.cfg.Role, valid.Round(time.Second), p.cfg.RenewBefore)
		}
	}

	return nil
}

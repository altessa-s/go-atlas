// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
)

var _ tlsproviders.Provider = (*issuingProvider)(nil)

// issuingProvider stands in for the Vault provider: it records the server names it is asked for and issues
// certificates valid for lifetime, after failing the first failures requests.
type issuingProvider struct {
	mu       sync.Mutex
	asked    []string
	failures int
	lifetime time.Duration
	bare     bool
}

func (p *issuingProvider) Type() tlsproviders.ProviderType { return tlsproviders.ProviderTypeVault }

func (p *issuingProvider) Close(context.Context) error { return nil }

func (p *issuingProvider) TLSConfig() (*tls.Config, error) {
	return &tls.Config{GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		p.mu.Lock()
		defer p.mu.Unlock()

		p.asked = append(p.asked, hello.ServerName)
		if hello.ServerName == "" {
			return nil, errors.New("missing server name")
		}
		if p.failures > 0 {
			p.failures--
			return nil, errors.New("vault is sealed")
		}

		if p.bare {
			return &tls.Certificate{}, nil
		}
		return &tls.Certificate{Leaf: &x509.Certificate{NotAfter: time.Now().Add(p.lifetime)}}, nil
	}}, nil
}

func (p *issuingProvider) names() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return strings.Join(p.asked, ",")
}

func role(renewBefore time.Duration) *tlsconfig.ProviderVault {
	return &tlsconfig.ProviderVault{
		Role: "api", CommonName: "api.example.svc.cluster.local", RenewBefore: renewBefore,
		SubjectAlternativeNames: []string{"api", "api.example", "api.example.svc", "api.example.svc.cluster.local"},
	}
}

func TestConfiguredNamesStandInTheCommonNameForUnknownNames(t *testing.T) {
	t.Parallel()

	issuer := &issuingProvider{lifetime: time.Hour}
	cfg, err := newConfiguredNames(issuer, role(time.Minute), true, false).TLSConfig()
	require.NoError(t, err)

	for _, name := range []string{"", "api.example", "API.Example.", "evil.example"} {
		_, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: name})
		require.NoError(t, err, name)
	}
	require.Equal(t, "api.example.svc.cluster.local,api.example,API.Example.,api.example.svc.cluster.local", issuer.names())
}

func TestWarm(t *testing.T) {
	t.Parallel()

	t.Run("every name once, after Vault recovers", func(t *testing.T) {
		t.Parallel()

		issuer := &issuingProvider{failures: 2, lifetime: 24 * time.Hour}
		require.NoError(t, newConfiguredNames(issuer, role(8*time.Hour), true, true).Warm(t.Context()))
		require.Equal(t, "api,api,api,api.example,api.example.svc,api.example.svc.cluster.local", issuer.names())
	})

	t.Run("Vault unreachable until the context ends", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), 1500*time.Millisecond)
		defer cancel()
		err := newConfiguredNames(&issuingProvider{failures: 1 << 20, lifetime: time.Hour}, role(time.Minute), true, true).Warm(ctx)
		require.ErrorContains(t, err, "vault is sealed")
		require.ErrorContains(t, err, "role api")
	})

	t.Run("role TTL within renewBefore", func(t *testing.T) {
		t.Parallel()

		err := newConfiguredNames(&issuingProvider{lifetime: time.Hour}, role(8*time.Hour), true, true).Warm(t.Context())
		require.ErrorContains(t, err, "not longer than renewBefore 8h0m0s")
	})

	t.Run("a certificate without a leaf", func(t *testing.T) {
		t.Parallel()

		err := newConfiguredNames(&issuingProvider{bare: true}, role(time.Minute), true, true).Warm(t.Context())
		require.ErrorContains(t, err, "no TLS certificate of api from Vault role api")
	})

	t.Run("without IssueOnBuild", func(t *testing.T) {
		t.Parallel()

		issuer := &issuingProvider{failures: 1 << 20}
		require.NoError(t, newConfiguredNames(issuer, role(time.Minute), true, false).Warm(t.Context()))
		require.Empty(t, issuer.names())
	})
}

// TestNamesPassThroughWithoutFallback asks Vault for every server name unless the fallback is on.
func TestNamesPassThroughWithoutFallback(t *testing.T) {
	t.Parallel()

	issuer := &issuingProvider{lifetime: time.Hour}
	cfg, err := newConfiguredNames(issuer, role(time.Minute), false, true).TLSConfig()
	require.NoError(t, err)
	_, err = cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "other.example"})
	require.NoError(t, err)
	require.Equal(t, "other.example", issuer.names())
}

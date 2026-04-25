// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsproviders

import (
	"context"
	"crypto/tls"
)

// Provider defines the interface for TLS certificate providers.
// Implementations provide TLS configurations from various sources like files,
// Vault, or Let's Encrypt with automatic certificate management.
type Provider interface {
	Type() ProviderType
	TLSConfig() (*tls.Config, error)
	// Close releases resources used by the provider.
	// The context controls the graceful shutdown timeout.
	Close(ctx context.Context) error
}

// ClientCertificate provides client certificates for mutual TLS authentication.
// Implementations can dynamically select certificates based on server requirements.
type ClientCertificate interface {
	// GetClientCertificate returns a client certificate for the given request info.
	GetClientCertificate(info *tls.CertificateRequestInfo) (*tls.Certificate, error)
}

// Certificate provides server certificates for TLS connections.
// Implementations can dynamically select certificates based on SNI or other client hello information.
type Certificate interface {
	// GetCertificate returns a certificate for the given client hello info.
	GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
}

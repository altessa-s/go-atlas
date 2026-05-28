// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsvault

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"time"

	"github.com/johanbrandhorst/certify"
	"github.com/johanbrandhorst/certify/issuers/vault"

	"github.com/altessa-s/go-atlas/security/tlsutils"

	corectx "github.com/altessa-s/go-atlas/core/context"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
)

// ocspTimeout is the maximum time allowed for OCSP staple fetching during TLS handshake.
const ocspTimeout = 5 * time.Second

// Vault is a TLS certificate provider that integrates with HashiCorp Vault.
// Use New to create a new instance.
type Vault struct {
	cc         *certify.Certify
	authMethod vault.AuthMethod
	options    *options

	// ocspCtx controls the lifecycle of OCSP refresh goroutines.
	// Canceled in Close() to stop all background refreshes.
	ocspCtx    context.Context
	ocspCancel context.CancelFunc

	// Frequently accessed fields copied from options
	ocspStapler tlsutils.OCSPStapler
	logger      *slog.Logger
}

// New creates a new Vault TLS provider with the given options.
// The provider connects to Vault PKI engine for dynamic certificate management.
//
// Example:
//
//	provider, err := tlsvault.New(
//		tlsvault.WithEndpoint("https://vault.example.com"),
//		tlsvault.WithStaticToken("s.xxxxxxxxx"),
//		tlsvault.WithRole("web-server"),
//	)
func New(opt ...Option) (*Vault, error) {
	opts, err := newOptions(opt...)
	if err != nil {
		return nil, err
	}

	// OCSP staple-refresh runs on a detached background context (canceled
	// by [Vault.Shutdown]) because it outlives any request-scoped
	// context the caller might pass through New. Termination is honest:
	// Shutdown invokes ocspCancel — no goroutine leak on stop.
	ocspCtx, ocspCancel := context.WithCancel(context.Background())
	v := &Vault{
		options:     opts,
		ocspCtx:     ocspCtx,
		ocspCancel:  ocspCancel,
		ocspStapler: opts.ocspStapler,
		logger:      opts.logger,
	}
	v.init()

	return v, nil
}

// GetCertificate returns a certificate for the given client hello info.
// It implements the tls.Config.GetCertificate callback.
// OCSP stapling is added automatically if an OCSP stapler is configured.
//
// Example:
//
//	config := &tls.Config{GetCertificate: provider.GetCertificate}
func (v *Vault) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert, err := v.cc.GetCertificate(hello)
	if err != nil {
		return nil, err
	}

	return v.withOCSPStaple(hello.Context(), cert)
}

// GetClientCertificate returns a client certificate for mTLS authentication.
// It implements the tls.Config.GetClientCertificate callback.
// OCSP stapling is added automatically if an OCSP stapler is configured.
//
// Example:
//
//	config := &tls.Config{GetClientCertificate: provider.GetClientCertificate}
func (v *Vault) GetClientCertificate(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	cert, err := v.cc.GetClientCertificate(info)
	if err != nil {
		return nil, err
	}

	return v.withOCSPStaple(info.Context(), cert)
}

func (v *Vault) withOCSPStaple(ctx context.Context, cert *tls.Certificate) (*tls.Certificate, error) {
	// Add OCSP stapling if enabled
	if v.ocspStapler != nil && cert != nil {
		// Apply timeout to prevent slow OCSP responses from blocking TLS handshake
		ocspCtx, cancel := corectx.ApplyTimeout(ctx, ocspTimeout)
		defer cancel()

		// GetOCSPStaple caches the certificate for scheduler-based refresh
		ocspResp, err := v.ocspStapler.GetOCSPStaple(ocspCtx, cert)
		if err == nil && len(ocspResp) > 0 {
			return tlsutils.CloneCertificateWithOCSPStaple(cert, ocspResp), nil
		}
	}

	return cert, nil
}

// TLSConfig returns a TLS configuration with Vault certificate management.
// The configuration uses secure defaults and enables HTTP/2 support.
//
// Example:
//
//	config, _ := provider.TLSConfig()
//	server := &http.Server{TLSConfig: config}
func (v *Vault) TLSConfig() (*tls.Config, error) {
	// Start with secure default TLS configuration
	tlsConfig := tlsutils.DefaultTLSConfig()

	// Add Vault-specific configuration
	tlsConfig.GetCertificate = v.GetCertificate
	tlsConfig.GetClientCertificate = v.GetClientCertificate
	tlsConfig.NextProtos = []string{
		"h2", "http/1.1", // enable HTTP/2
	}

	return tlsConfig, nil
}

// Close cleans up resources used by the Vault provider.
// It cancels OCSP refresh goroutines and closes the authentication method if present.
// The context controls the graceful shutdown timeout for auth methods that support it.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	provider.Close(ctx)
func (v *Vault) Close(ctx context.Context) error {
	// Cancel OCSP context to stop all refresh goroutines
	v.ocspCancel()

	// Close auth method if it supports closing
	if v.authMethod != nil {
		type contextCloser interface {
			Close(context.Context) error
		}
		if cc, ok := v.authMethod.(contextCloser); ok {
			return cc.Close(ctx)
		}
		if cl, ok := v.authMethod.(io.Closer); ok {
			return cl.Close()
		}
	}

	return nil
}

// Type returns the provider type identifier.
// Always returns ProviderTypeVault.
//
// Example:
//
//	fmt.Println(provider.Type()) // "vault"
func (v *Vault) Type() tlsproviders.ProviderType {
	return tlsproviders.ProviderTypeVault
}

func (v *Vault) init() {
	switch tt := v.options.token.(type) {
	case string:
		v.authMethod = vault.ConstantToken(tt)
	case *Token:
		v.authMethod = &vault.RenewingToken{
			Initial:     tt.initialToken,
			RenewBefore: tt.renewBefore,
			TimeToLive:  tt.ttl,
		}
	}

	v.cc = &certify.Certify{
		CommonName:  v.options.commonName,
		Issuer:      v.issuer(),
		Cache:       certify.NewMemCache(),
		CertConfig:  &certify.CertConfig{KeyGenerator: NewRSAGenerator()},
		RenewBefore: v.options.renewBefore,
		Logger:      NewLogger(v.logger),
	}

	if v.options.cacheDir != "" {
		v.cc.Cache = certify.DirCache(v.options.cacheDir)
	}

	if v.options.subjectAlternativeNames != nil {
		v.cc.CertConfig.SubjectAlternativeNames = v.options.subjectAlternativeNames
	}

	if v.options.ipSubjectAlternativeNames != nil {
		v.cc.CertConfig.IPSubjectAlternativeNames = v.options.ipSubjectAlternativeNames
	}
}

func (v *Vault) issuer() *vault.Issuer {
	if v.options.vc != nil {
		return vault.FromClient(v.options.vc, v.options.role)
	}

	return &vault.Issuer{
		AuthMethod: v.authMethod,
		Role:       v.options.role,
		URL:        v.options.endpoint,
	}
}

var _ tlsproviders.ClientCertificate = (*Vault)(nil)
var _ tlsproviders.Certificate = (*Vault)(nil)
var _ tlsproviders.Provider = (*Vault)(nil)

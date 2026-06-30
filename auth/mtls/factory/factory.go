// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"

	"github.com/altessa-s/go-atlas/config"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// ekuByName maps configuration EKU names to their x509 values.
var ekuByName = coremaps.NewImmutableMap(map[string]x509.ExtKeyUsage{
	"clientAuth": x509.ExtKeyUsageClientAuth,
	"serverAuth": x509.ExtKeyUsageServerAuth,
	"any":        x509.ExtKeyUsageAny,
})

// Builder turns a [config.MTLS] into the [auth/mtls] validator options it
// describes. It is the config-driven counterpart to assembling those options by
// hand, giving the mTLS subsystem the same config→component path the OPA and
// scope factories provide.
//
// Only the certificate-validation policy is declarative. The identity function,
// audit recorder, and transport label stay at the call site, so [Builder.Options]
// returns options the transport adapter combines with its own — it does not build
// a finished authenticator with a transport label baked in.
type Builder struct {
	cfg *config.MTLS
}

// New creates a [Builder] for the given configuration. A nil cfg is accepted;
// the error surfaces at [Builder.Options] / [Builder.Authenticator] time.
func New(cfg *config.MTLS) *Builder {
	return &Builder{cfg: cfg}
}

// Options returns the validator options described by the configuration: an
// expiry re-check (when CheckExpiry) and a trust-domain pin (when TrustDomains is
// non-empty). Pass them to a transport adapter's AuthFunc/Middleware alongside
// any identity or audit options:
//
//	opts, err := factory.New(&cfg.MTLS).Options()
//	authFn := grpcmtls.AuthFunc(append(opts, coremtls.WithAudit(rec, subjectOf))...)
func (b *Builder) Options() ([]coremtls.Option, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("mtls/factory: configuration is required")
	}
	var opts []coremtls.Option
	if b.cfg.CheckExpiry {
		opts = append(opts, coremtls.WithValidator(coremtls.ExpiryValidator(nil, b.cfg.ExpiryLeeway)))
	}
	if len(b.cfg.TrustDomains) > 0 {
		opts = append(opts, coremtls.WithValidator(coremtls.TrustDomainValidator(b.cfg.TrustDomains...)))
	}
	if len(b.cfg.AllowedSubjectCNs) > 0 {
		opts = append(opts, coremtls.WithValidator(coremtls.SubjectValidator(cnNames(b.cfg.AllowedSubjectCNs)...)))
	}
	if len(b.cfg.AllowedIssuerCNs) > 0 {
		opts = append(opts, coremtls.WithValidator(coremtls.IssuerValidator(cnNames(b.cfg.AllowedIssuerCNs)...)))
	}
	if len(b.cfg.IssuerKeyIDs) > 0 {
		keyIDs := make([][]byte, 0, len(b.cfg.IssuerKeyIDs))
		for _, h := range b.cfg.IssuerKeyIDs {
			raw, err := hex.DecodeString(h)
			if err != nil {
				return nil, fmt.Errorf("mtls/factory: invalid issuer key id %q: %w", h, err)
			}
			keyIDs = append(keyIDs, raw)
		}
		opts = append(opts, coremtls.WithValidator(coremtls.AuthorityKeyIDValidator(keyIDs...)))
	}
	if len(b.cfg.AllowedDNSNames) > 0 {
		opts = append(opts, coremtls.WithValidator(coremtls.DNSNameValidator(b.cfg.AllowedDNSNames...)))
	}
	if len(b.cfg.RequiredEKUs) > 0 {
		ekus, err := parseEKUs(b.cfg.RequiredEKUs)
		if err != nil {
			return nil, err
		}
		opts = append(opts, coremtls.WithValidator(coremtls.EKUValidator(ekus...)))
	}
	return opts, nil
}

// parseEKUs maps configuration EKU names to their x509 values.
func parseEKUs(names []string) ([]x509.ExtKeyUsage, error) {
	out := make([]x509.ExtKeyUsage, 0, len(names))
	for _, n := range names {
		eku, ok := ekuByName.Get(n)
		if !ok {
			return nil, fmt.Errorf("mtls/factory: unknown EKU %q", n)
		}
		out = append(out, eku)
	}
	return out, nil
}

// cnNames maps a list of CommonNames to subset-match [pkix.Name] values.
func cnNames(cns []string) []pkix.Name {
	names := make([]pkix.Name, 0, len(cns))
	for _, cn := range cns {
		names = append(names, pkix.Name{CommonName: cn})
	}
	return names
}

// Authenticator builds a standalone [coremtls.Authenticator] from the
// configuration plus any extra options (identity, audit, transport label). Use
// it for non-transport callers; transport adapters take [Builder.Options]
// instead so they can set their own transport label.
func (b *Builder) Authenticator(extra ...coremtls.Option) (*coremtls.Authenticator, error) {
	opts, err := b.Options()
	if err != nil {
		return nil, err
	}
	return coremtls.NewAuthenticator(append(opts, extra...)...), nil
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"encoding/hex"
	"fmt"
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// DefaultMTLSExpiryLeeway is the default clock-skew tolerance for the mTLS
// certificate-expiry re-check.
const DefaultMTLSExpiryLeeway = 30 * time.Second

// MTLS is the configuration for the mutual-TLS authentication validators built
// by [auth/mtls/factory.New]. It drives only the certificate-validation policy;
// the identity function and audit recorder are wired in code, and the TLS
// credentials that require and verify the client certificate are configured on
// the server through [security/tlsutils].
//
// Example:
//
//	mtls:
//	  trustDomains: ["example.org"]
//	  checkExpiry: true
//	  expiryLeeway: 30s
type MTLS struct {
	// TrustDomains pins the accepted SPIFFE trust domains. Empty accepts any
	// trust domain (no trust-domain validator is added).
	TrustDomains []string `yaml:"trustDomains"`

	// CheckExpiry re-validates the certificate validity window on every call, so
	// a long-lived connection that outlives its certificate is rejected. The TLS
	// handshake already checks expiry at connection time. Defaults to true.
	CheckExpiry bool `yaml:"checkExpiry" default:"true"`

	// ExpiryLeeway is the clock-skew tolerance for the expiry re-check. Applies
	// when CheckExpiry is true. Defaults to 30s.
	ExpiryLeeway time.Duration `yaml:"expiryLeeway" default:"30s"`

	// AllowedSubjectCNs pins the accepted certificate Subject CommonNames, for
	// classic-PKI identity carried in the DN rather than a SPIFFE URI. Empty
	// disables the check.
	AllowedSubjectCNs []string `yaml:"allowedSubjectCNs"`

	// AllowedIssuerCNs pins the accepted issuer (CA) CommonNames, narrowing the
	// handshake's chain trust to specific issuers. Empty disables the check.
	AllowedIssuerCNs []string `yaml:"allowedIssuerCNs"`

	// IssuerKeyIDs pins the accepted issuing-CA Authority Key IDs, hex-encoded.
	// The most robust CA pin. Empty disables the check.
	IssuerKeyIDs []string `yaml:"issuerKeyIDs"`

	// AllowedDNSNames pins the accepted DNS Subject Alternative Names (wildcard
	// SANs honored). Empty disables the check.
	AllowedDNSNames []string `yaml:"allowedDNSNames"`

	// RequiredEKUs requires the certificate to assert these extended key usages.
	// Accepted values: "clientAuth", "serverAuth", "any". Empty disables the check.
	RequiredEKUs []string `yaml:"requiredEKUs"`
}

// Validate validates the mTLS configuration.
func (c *MTLS) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.ExpiryLeeway, validation.When(c.CheckExpiry, ozzo_rules.Duration())),
		validation.Field(&c.IssuerKeyIDs, validation.Each(validation.By(validHexString))),
		validation.Field(&c.RequiredEKUs, validation.Each(validation.In("clientAuth", "serverAuth", "any"))),
	)
}

// validHexString reports whether value is a hex-decodable string.
func validHexString(value any) error {
	s, _ := value.(string)
	if _, err := hex.DecodeString(s); err != nil {
		return fmt.Errorf("must be hex-encoded: %w", err)
	}
	return nil
}

// DefaultMTLS returns an MTLS configuration with default values.
func DefaultMTLS() MTLS {
	return MTLS{
		CheckExpiry:  true,
		ExpiryLeeway: DefaultMTLSExpiryLeeway,
	}
}

// IsEnabled reports whether the configuration is present.
func (c *MTLS) IsEnabled() bool {
	return c != nil
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"

	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// DefaultMTLSExpiryLeeway is the default clock-skew tolerance for the mTLS
// certificate-expiry re-check.
const DefaultMTLSExpiryLeeway = 30 * time.Second

// MTLS is the configuration for the mutual-TLS authentication validators built
// by [factory/auth/mtls.New]. It drives only the certificate-validation policy;
// the identity function and audit recorder are wired in code, and the TLS
// credentials that require and verify the client certificate are configured on
// the server through [github.com/altessa-s/go-tlsutils].
//
// Example:
//
//	mtls:
//	  trustDomains: ["example.org"]
//	  checkExpiry: true
//	  expiryLeeway: 30s
type MTLS struct {
	// TrustDomains pins the accepted SPIFFE trust domains. The factory requires
	// either a non-empty list or AllowAnyTrustDomain: it refuses to build
	// options from a configuration that makes neither decision. Every entry
	// must be a SPIFFE trust domain name: 1 to 255 lower-case letters, digits,
	// dots, dashes and underscores. An empty entry — an unset substitution, for
	// example — is a validation error.
	TrustDomains []string `yaml:"trustDomains"`

	// AllowAnyTrustDomain accepts a certificate from any SPIFFE trust domain
	// when TrustDomains is empty, leaving the TLS trust pool as the whole trust
	// decision: a shared or federated CA can then issue an accepted certificate
	// for a foreign trust domain. It has no effect when TrustDomains is set.
	// Certificates without a SPIFFE ID (classic PKI, identity in the DN) need
	// it; pin them with AllowedSubjectCNs, AllowedIssuerCNs or IssuerKeyIDs.
	// Defaults to false.
	AllowAnyTrustDomain bool `yaml:"allowAnyTrustDomain" default:"false"`

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
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.TrustDomains, validation.Each(validation.By(validTrustDomain))),
		validation.Field(&c.ExpiryLeeway, validation.When(c.CheckExpiry, validationconfig.PositiveDuration())),
		validation.Field(&c.AllowedSubjectCNs, validation.Each(validation.Required, validation.By(nonBlankValue))),
		validation.Field(&c.AllowedIssuerCNs, validation.Each(validation.Required, validation.By(nonBlankValue))),
		validation.Field(&c.AllowedDNSNames, validation.Each(validation.Required, validation.By(nonBlankValue))),
		validation.Field(&c.IssuerKeyIDs, validation.Each(validation.Required, validation.By(validHexString))),
		validation.Field(&c.RequiredEKUs, validation.Each(validation.In("clientAuth", "serverAuth", "any"))),
	)
}

// maxTrustDomainLen is the longest SPIFFE trust domain name.
const maxTrustDomainLen = 255

// trustDomainPattern is the character set of a SPIFFE trust domain name.
var trustDomainPattern = regexp.MustCompile(`^[a-z0-9._-]+$`)

// validTrustDomain reports whether value is a SPIFFE trust domain name. An
// empty value is an error: ozzo skips empty strings in built-in rules, and an
// empty entry must not pass for a pinned domain.
func validTrustDomain(value any) error {
	s, _ := value.(string)
	if s == "" {
		return errors.New("must not be empty")
	}
	if len(s) > maxTrustDomainLen || !trustDomainPattern.MatchString(s) {
		return fmt.Errorf("must be a SPIFFE trust domain: 1 to %d lower-case letters, digits, dots, dashes and underscores",
			maxTrustDomainLen)
	}
	return nil
}

// validHexString reports whether value is a hex-decodable string.
func validHexString(value any) error {
	s, _ := value.(string)
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return errors.New("must be hex encoded")
	}
	if len(decoded) == 0 {
		return errors.New("must not decode to an empty key ID")
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

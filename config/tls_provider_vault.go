// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	"github.com/go-ozzo/ozzo-validation/is"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// TlsProviderVault configures HashiCorp Vault PKI certificate provider.
// It integrates with Vault's PKI secrets engine to dynamically generate and manage
// certificates with automatic renewal. Supports Subject Alternative Names (SANs)
// for multi-domain and IP-based certificates.
type TlsProviderVault struct {
	// CommonName sets the certificate's common name (primary domain or service name)
	CommonName string `yaml:"commonName"`
	// Role specifies the PKI role in Vault that defines certificate constraints and policies
	Role string `yaml:"role"`
	// RenewBefore specifies when to renew the certificate before expiration
	RenewBefore time.Duration `yaml:"renewBefore" default:"30m"`
	// SubjectAlternativeNames contains additional DNS names for the certificate
	SubjectAlternativeNames []string `yaml:"subjectAlternativeNames"`
	// IPSubjectAlternativeNames contains IP addresses to include in the certificate
	IPSubjectAlternativeNames []string `yaml:"ipSubjectAlternativeNames"`
}

// Validate checks that the Vault provider configuration is valid.
// It ensures that required fields (role, common name) are provided and validates
// that IP addresses in Subject Alternative Names are properly formatted.
//
// Returns an error if validation fails.
func (l *TlsProviderVault) Validate() error {
	return ValidateStruct(l,
		validation.Field(&l.CommonName, validation.Required),
		validation.Field(&l.Role, validation.Required),
		validation.Field(&l.RenewBefore, ozzo_rules.DurationOrZero()),
		validation.Field(&l.SubjectAlternativeNames, validation.When(len(l.SubjectAlternativeNames) > 0,
			validation.Each(validation.Required))),
		validation.Field(&l.IPSubjectAlternativeNames, validation.When(len(l.IPSubjectAlternativeNames) > 0,
			validation.Each(is.IP))),
	)
}

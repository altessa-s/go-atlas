// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import validation "github.com/go-ozzo/ozzo-validation/v4"

// TlsProviderType defines the type of Tls certificate provider.
// Different providers offer different methods for obtaining and managing Tls certificates.
type TlsProviderType string

const (
	// TlsProviderTypeFile loads certificates from local PEM files.
	TlsProviderTypeFile TlsProviderType = "file"

	// TlsProviderTypeLetsEncrypt obtains certificates automatically via the ACME protocol.
	TlsProviderTypeLetsEncrypt TlsProviderType = "letsencrypt"

	// TlsProviderTypeVault provisions certificates from a HashiCorp Vault PKI backend.
	TlsProviderTypeVault TlsProviderType = "vault"
)

// TlsProvider represents the configuration for Tls certificate providers.
// It contains provider-specific settings for obtaining Tls certificates.
// Only the configuration matching the specified provider type is required.
type TlsProvider struct {
	// File contains file-based certificate provider settings.
	// Used when certificates are loaded from local files.
	File *TlsProviderFile `yaml:"file" default:"-"`

	// LetsEncrypt contains Let's Encrypt ACME provider settings.
	// Used for automatic certificate provisioning via ACME protocol.
	LetsEncrypt *TlsProviderLetsEncrypt `yaml:"letsencrypt" default:"-"`

	// Vault contains HashiCorp Vault provider settings.
	// Used for certificate provisioning via Vault PKI backend.
	Vault *TlsProviderVault `yaml:"vault" default:"-"`
}

// Validate performs validation on the Tls provider configuration.
// It ensures that provider-specific configurations are valid when present.
//
// Returns an error if any validation rules fail.
func (c *TlsProvider) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.File, validation.Required.When(c.File != nil)),
		validation.Field(&c.LetsEncrypt, validation.Required.When(c.LetsEncrypt != nil)),
		validation.Field(&c.Vault, validation.Required.When(c.Vault != nil)),
	)
}

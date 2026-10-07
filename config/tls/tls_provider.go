// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsconfig

import (
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ProviderType defines the type of Tls certificate provider.
// Different providers offer different methods for obtaining and managing Tls certificates.
type ProviderType string

const (
	// ProviderTypeFile loads certificates from local PEM files.
	ProviderTypeFile ProviderType = "file"

	// ProviderTypeLetsEncrypt obtains certificates automatically via the ACME protocol.
	ProviderTypeLetsEncrypt ProviderType = "letsencrypt"

	// ProviderTypeVault provisions certificates from a HashiCorp Vault PKI backend.
	ProviderTypeVault ProviderType = "vault"

	// ProviderTypeS3 downloads certificates from S3-compatible storage.
	ProviderTypeS3 ProviderType = "s3"
)

// Provider represents the configuration for Tls certificate providers.
// It contains provider-specific settings for obtaining Tls certificates.
// Only the configuration matching the specified provider type is required.
type Provider struct {
	// File contains file-based certificate provider settings.
	// Used when certificates are loaded from local files.
	File *ProviderFile `yaml:"file" default:"-"`

	// LetsEncrypt contains Let's Encrypt ACME provider settings.
	// Used for automatic certificate provisioning via ACME protocol.
	LetsEncrypt *ProviderLetsEncrypt `yaml:"letsencrypt" default:"-"`

	// Vault contains HashiCorp Vault provider settings.
	// Used for certificate provisioning via Vault PKI backend.
	Vault *ProviderVault `yaml:"vault" default:"-"`

	// S3 contains S3-based certificate provider settings.
	// Used when certificates are downloaded from S3-compatible storage.
	S3 *ProviderS3 `yaml:"s3" default:"-"`

	// OCSP configures the OCSP stapler attached to file / Vault / S3
	// providers. When nil (or Enabled=false) the builder does not
	// auto-construct a stapler; programmatic callers may still inject
	// one via ProvidersBuilder.UseOcspStapler.
	OCSP *ProviderOCSP `yaml:"ocsp" default:"-"`
}

// Validate performs validation on the Tls provider configuration.
// It ensures that provider-specific configurations are valid when present.
//
// Returns an error if any validation rules fail.
func (c *Provider) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.File, validation.Required.When(c.File != nil)),
		validation.Field(&c.LetsEncrypt, validation.Required.When(c.LetsEncrypt != nil)),
		validation.Field(&c.Vault, validation.Required.When(c.Vault != nil)),
		validation.Field(&c.S3, validation.Required.When(c.S3 != nil)),
		validation.Field(&c.OCSP, validation.NilOrNotEmpty),
	)
}

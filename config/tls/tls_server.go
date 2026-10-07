// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsconfig

import (
	"github.com/altessa-s/go-atlas/config/internal/utils"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ClientAuth represents the configuration for client certificate authentication.
// It defines how the server should authenticate client certificates in mutual Tls scenarios.
type ClientAuth struct {
	// CACerts is a list of paths to Certificate Authority certificate files.
	// Used to verify client certificates during mutual Tls authentication.
	CACerts []string `yaml:"caCerts"`

	// AllowCertAuth enables client certificate authentication.
	// When true, clients must present valid certificates signed by the configured CAs.
	AllowCertAuth bool `yaml:"allowCertAuth"`
}

// Normalize processes the Tls client auth configuration by resolving CA certificate file paths.
// It attempts to locate CA certificate files using the findFile helper.
// This method should be called after loading configuration.
func (c *ClientAuth) Normalize() {
	for i, ca := range c.CACerts {
		c.CACerts[i] = utils.FindFile(ca)
	}
}

// Validate performs validation on the Tls client auth configuration.
// It verifies that all CA certificate files exist.
//
// Returns an error if any validation rules fail.
func (c *ClientAuth) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.CACerts, validation.Each(ozzo_rules.File())),
	)
}

// Server represents the configuration for Tls server settings.
// It defines the certificate provider and minimum Tls version for server connections.
type Server struct {
	// Provider specifies how Tls certificates are obtained.
	// Defaults to "file" for file-based certificates.
	// Supported types: file, letsencrypt, vault.
	Provider ProviderType `yaml:"provider" default:"file"`

	// MinTLSVersion specifies the minimum Tls version to accept.
	// Defaults to "1.2". Supported values: "1.2", "1.3".
	MinTLSVersion string `yaml:"minVersion" default:"1.2"`
}

// Validate performs validation on the Tls server configuration.
// It validates the provider type and minimum Tls version settings.
//
// Returns an error if any validation rules fail.
func (c *Server) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Provider,
			ozzo_rules.OneOf(ProviderTypeFile, ProviderTypeLetsEncrypt, ProviderTypeVault, ProviderTypeS3)),
		validation.Field(&c.MinTLSVersion, validation.Required, ozzo_rules.OneOf("1.2", "1.3")),
	)
}

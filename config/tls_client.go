// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"github.com/altessa-s/go-atlas/config/internal/utils"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// TlsClient represents the configuration for Tls client connections.
// It contains certificates, private keys, and CA certificates needed for
// establishing secure Tls connections as a client.
type TlsClient struct {
	// Certificate is the path to the client certificate file.
	// Used for mutual Tls authentication.
	Certificate string `yaml:"certificate"`

	// PrivateKey is the path to the client private key file.
	// Must correspond to the client certificate.
	PrivateKey string `yaml:"privateKey"`

	// PrivateKeyPassword is the password for encrypted private key files.
	// Leave empty if the private key is not encrypted.
	PrivateKeyPassword Secret `yaml:"privateKeyPassword"`

	// CACerts is a list of paths to Certificate Authority certificate files.
	// Used to verify the server's certificate.
	CACerts []string `yaml:"caCerts"`

	// ServerName is the expected server name for certificate verification.
	// Used to verify the server's certificate matches the expected hostname.
	ServerName string `yaml:"serverName"`

	// SkipVerify disables server certificate verification.
	// WARNING: Setting this to true makes connections insecure.
	// Defaults to false.
	SkipVerify bool `yaml:"skipVerifyServerName" default:"false"`
}

// Normalize processes the Tls client configuration by resolving file paths.
// It attempts to locate certificate and key files using the findFile helper.
// This method should be called after loading configuration.
func (c *TlsClient) Normalize() {
	c.Certificate = utils.FindFile(c.Certificate)
	c.PrivateKey = utils.FindFile(c.PrivateKey)

	for i, ca := range c.CACerts {
		c.CACerts[i] = utils.FindFile(ca)
	}
}

// Validate performs validation on the Tls client configuration.
// It verifies that certificate, private key, and CA certificate files exist.
//
// Returns an error if any validation rules fail.
func (c *TlsClient) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Certificate, ozzo_rules.File()),
		validation.Field(&c.PrivateKey, ozzo_rules.File()),
		validation.Field(&c.CACerts, validation.Each(ozzo_rules.File())),
	)
}

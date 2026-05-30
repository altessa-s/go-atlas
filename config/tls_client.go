// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"os"
	"strings"

	"github.com/altessa-s/go-atlas/config/internal/utils"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// EnvAllowInsecureTLS is the environment variable that must be set to "true"
// to permit SkipVerify in TLS client configurations. When SkipVerify is true
// in the config but this variable is absent or not "true", Normalize resets
// SkipVerify to false.
const EnvAllowInsecureTLS = "ATLAS_ALLOW_INSECURE_TLS"

// TLSSkipVerifyMode controls how the TLS factory reacts when a
// [TlsClient] config has SkipVerify=true. Follows the project's
// safety-mode pattern (mirror of plugins.SignatureMode and
// oidc.JWKSFailureMode) so operators have a uniform escape hatch shape.
type TLSSkipVerifyMode string

const (
	// TLSSkipVerifyModeEnforce rejects SkipVerify=true at TLS-config build
	// time with an error. This is the production-safe default — an
	// operator who needs to disable verification (lab, debugging) must
	// opt in explicitly via Warn or Disabled rather than silently
	// shipping an insecure client.
	TLSSkipVerifyModeEnforce TLSSkipVerifyMode = "enforce"

	// TLSSkipVerifyModeWarn allows SkipVerify=true but logs a loud
	// warning on every CreateClientConfig call. Use during deliberate
	// canary windows where downstream isn't yet exposing a trusted CA.
	TLSSkipVerifyModeWarn TLSSkipVerifyMode = "warn"

	// TLSSkipVerifyModeDisabled silently allows SkipVerify=true with no
	// log. Intended exclusively for unit/integration tests that wire up
	// localhost-only fixtures; using it in any deployable artifact is a
	// review blocker.
	TLSSkipVerifyModeDisabled TLSSkipVerifyMode = "disabled"
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

	// SkipVerifyMode controls how the TLS factory handles a SkipVerify=true
	// config — see [TLSSkipVerifyMode] for the semantics. The YAML loader
	// fills in "enforce" via the default tag; programmatic callers must
	// set it explicitly (the factory rejects an empty/unknown value with
	// [factory.ErrInsecureSkipVerifyRejected] rather than inheriting a
	// silent fallback). Pair this with the env-var gate
	// ([EnvAllowInsecureTLS]) — Normalize zeros out SkipVerify when the
	// env var is missing, and SkipVerifyMode then gates the residual case
	// where the env var IS set.
	SkipVerifyMode TLSSkipVerifyMode `yaml:"skipVerifyMode" default:"enforce"`
}

// Normalize processes the Tls client configuration by resolving file paths.
// It attempts to locate certificate and key files using the findFile helper.
// If SkipVerify is true but the ATLAS_ALLOW_INSECURE_TLS environment variable
// is not set to "true", SkipVerify is reset to false.
// An empty SkipVerifyMode is defaulted to the production-safe
// [TLSSkipVerifyModeEnforce] so programmatic callers that build TlsClient by
// hand match the YAML loader (which applies the default:"enforce" tag) instead
// of failing the factory with a confusing empty-mode error.
// This method should be called after loading configuration.
func (c *TlsClient) Normalize() {
	c.Certificate = utils.FindFile(c.Certificate)
	c.PrivateKey = utils.FindFile(c.PrivateKey)

	for i, ca := range c.CACerts {
		c.CACerts[i] = utils.FindFile(ca)
	}

	if c.SkipVerifyMode == "" {
		c.SkipVerifyMode = TLSSkipVerifyModeEnforce
	}

	if c.SkipVerify && !strings.EqualFold(os.Getenv(EnvAllowInsecureTLS), "true") {
		c.SkipVerify = false
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

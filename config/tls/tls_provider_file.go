// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsconfig

import (
	"github.com/altessa-s/go-atlas/core/types/redacted"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ProviderFile configures file-based Tls certificate provider.
// It loads certificates and private keys from the filesystem, suitable for
// static certificate deployments where certificates are managed externally.
// Supports password-protected private keys for enhanced security.
type ProviderFile struct {
	// Certificate file path containing the PEM-encoded certificate
	Certificate string `yaml:"certificate"`
	// PrivateKey file path containing the PEM-encoded private key
	PrivateKey string `yaml:"privateKey"`
	// PrivateKeyPassword for encrypted private keys (optional)
	PrivateKeyPassword redacted.RedactedString `yaml:"privateKeyPassword"`
}

// Validate checks that the file provider configuration is valid.
// It ensures that both certificate and private key file paths are provided
// and that the files exist and are accessible.
//
// Returns an error if validation fails.
func (c *ProviderFile) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Certificate, ozzo_rules.File()),
		validation.Field(&c.PrivateKey, ozzo_rules.File()),
	)
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import (
	"time"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// TLS configures Tls settings specifically for Http servers.
// It embeds TlsServer configuration to provide certificate management,
// Tls versions, and cipher suite configuration for Http endpoints.
// It also supports Http Strict Transport Security (HSTS) configuration.
type TLS struct {
	*tlsconfig.Server `yaml:",inline"`
	// StrictTransport configures Http Strict Transport Security (HSTS) headers
	StrictTransport *STS `yaml:"sts" default:"-"`
}

// Validate checks that the HTTPTLS configuration is valid.
// It ensures that the embedded TlsServer configuration is present and valid,
// and validates the HSTS configuration if provided.
//
// Returns an error if validation fails.
func (t *TLS) Validate() error {
	return validationconfig.ValidateStruct(t,
		validation.Field(&t.Server, validation.Required),
		validation.Field(&t.StrictTransport, validation.Required.When(t.StrictTransport != nil)),
	)
}

// STS configures Http Strict Transport Security (HSTS) headers.
// HSTS instructs browsers to only access the site over HTTPS for a specified duration.
type STS struct {
	// MaxAge specifies how long browsers should enforce HTTPS-only access
	// Default is 8760h (1 year)
	MaxAge time.Duration `yaml:"maxAge" default:"8760h"`
}

// Validate checks that the STS configuration is valid.
// It ensures that MaxAge is at least 1 hour as recommended by security standards.
//
// Returns an error if validation fails.
func (s *STS) Validate() error {
	return validationconfig.ValidateStruct(s,
		validation.Field(&s.MaxAge, ozzo_rules.DurationWithLimit("1h", "-1")),
	)
}

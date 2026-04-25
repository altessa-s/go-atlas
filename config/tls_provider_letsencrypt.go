// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	"github.com/go-ozzo/ozzo-validation/v4/is"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// TlsProviderLetsEncrypt configures Let's Encrypt ACME certificate provider.
// It automatically obtains and renews Tls certificates from Let's Encrypt using domain validation.
// Supports automatic Http server for ACME challenge handling.
type TlsProviderLetsEncrypt struct {
	// Email address for Let's Encrypt account registration and notifications
	Email string `yaml:"email"`
	// Domain names to include in the certificate (first domain becomes the common name)
	Domain []string `yaml:"domain"`
	// RenewBefore specifies when to renew the certificate before expiration
	RenewBefore time.Duration `yaml:"renewBefore" default:"30m"`
	// StartInternalServer enables automatic Http server for ACME challenge handling
	StartInternalServer bool `yaml:"startInternalServer"`
	// ListenAddress specifies the address for the internal ACME challenge server
	ListenAddress *string `yaml:"listenAddress" default:"0.0.0.0:8080"`
}

// Validate checks that the Let's Encrypt provider configuration is valid.
// It ensures that required fields (email, domains) are provided, validates email format,
// domain format, and listen address when internal server is enabled.
//
// Returns an error if validation fails.
func (l *TlsProviderLetsEncrypt) Validate() error {
	return ValidateStruct(l,
		validation.Field(&l.Domain, validation.Required, validation.Each(is.Domain)),
		validation.Field(&l.Email, validation.Required, is.Email),
		validation.Field(&l.RenewBefore, ozzo_rules.DurationOrZero()),
		validation.Field(&l.ListenAddress, validation.When(l.StartInternalServer, ozzo_rules.ListenAddress())),
	)
}

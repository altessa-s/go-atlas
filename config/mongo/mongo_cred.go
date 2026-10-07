// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoconfig

import (
	"github.com/altessa-s/go-atlas/core/types/redacted"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// AuthMechanismType represents the authentication mechanism type for MongoDB.
// MongoDB supports several authentication mechanisms for different security requirements.
type AuthMechanismType string

const (
	// AuthMechanismTypePLAIN uses PLAIN (LDAP) authentication. Requires TLS.
	AuthMechanismTypePLAIN AuthMechanismType = "PLAIN"

	// AuthMechanismTypeX509 uses X.509 certificate authentication.
	// The [Config.TLS] field must be configured when this mechanism is selected.
	AuthMechanismTypeX509 AuthMechanismType = "MONGODB-X509"

	// AuthMechanismTypeSCRAMSHA256 uses SCRAM-SHA-256 (default, recommended).
	AuthMechanismTypeSCRAMSHA256 AuthMechanismType = "SCRAM-SHA-256"

	// AuthMechanismTypeSCRAMSHA1 uses the legacy SCRAM-SHA-1 mechanism.
	AuthMechanismTypeSCRAMSHA1 AuthMechanismType = "SCRAM-SHA-1"
)

// String returns the string representation of the authentication mechanism type.
func (mt AuthMechanismType) String() string {
	return string(mt)
}

// PLAINCredentials represents the credentials for the PLAIN authentication mechanism.
// PLAIN authentication sends credentials as plain text and should only be used
// over secure connections (TLS).
type PLAINCredentials struct {
	Username string                  `yaml:"username"`
	Password redacted.RedactedString `yaml:"password"`
}

// Validate validates the credentials for the PLAIN authentication mechanism.
// It ensures both username and password are provided.
//
// Returns an error if any validation rules fail.
func (m *PLAINCredentials) Validate() error {
	return validationconfig.ValidateStruct(m,
		validation.Field(&m.Username, validation.Required),
		validation.Field(&m.Password, validation.Required),
	)
}

// SCRAMCredentials represents the credentials for the SCRAM authentication mechanism.
// SCRAM (Salted Challenge Response Authentication Mechanism) is the default and
// recommended authentication mechanism for MongoDB.
type SCRAMCredentials struct {
	Username   string                  `yaml:"username"`
	Password   redacted.RedactedString `yaml:"password"`
	AuthSource string                  `yaml:"authSource" default:"admin"`
}

// Validate validates the credentials for the SCRAM authentication mechanism.
// It ensures username, password, and authentication source are provided.
//
// Returns an error if any validation rules fail.
func (m *SCRAMCredentials) Validate() error {
	return validationconfig.ValidateStruct(m,
		validation.Field(&m.Username, validation.Required),
		validation.Field(&m.Password, validation.Required),
		validation.Field(&m.AuthSource, validation.Required),
	)
}

// Credentials represents the credentials for the MongoDB connection.
// It contains the authentication mechanism type and corresponding credential details.
// Only the credentials matching the specified mechanism are required.
type Credentials struct {
	AuthMechanism AuthMechanismType `yaml:"authMechanism" default:"SCRAM-SHA-256"`
	Plain         *PLAINCredentials `yaml:"plain" default:"-"`
	Scram         *SCRAMCredentials `yaml:"scram" default:"-"`
}

// Validate validates the credentials for the MongoDB connection.
// It ensures the authentication mechanism is valid and that the corresponding
// credential fields are properly configured.
//
// Returns an error if any validation rules fail.
func (m *Credentials) Validate() error {
	return validationconfig.ValidateStruct(m,
		validation.Field(&m.AuthMechanism, ozzo_rules.OneOf(
			AuthMechanismTypeSCRAMSHA1,
			AuthMechanismTypeSCRAMSHA256,
			AuthMechanismTypeX509,
			AuthMechanismTypePLAIN)),
		validation.Field(&m.Plain, validation.Required.When(m.AuthMechanism == AuthMechanismTypePLAIN)),
		validation.Field(&m.Scram, validation.Required.When(m.AuthMechanism == AuthMechanismTypeSCRAMSHA1 ||
			m.AuthMechanism == AuthMechanismTypeSCRAMSHA256)),
	)
}

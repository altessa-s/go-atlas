// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// MongoAuthMechanismType represents the authentication mechanism type for MongoDB.
// MongoDB supports several authentication mechanisms for different security requirements.
type MongoAuthMechanismType string

const (
	// MongoAuthMechanismTypePLAIN uses PLAIN (LDAP) authentication. Requires TLS.
	MongoAuthMechanismTypePLAIN MongoAuthMechanismType = "PLAIN"

	// MongoAuthMechanismTypeX509 uses X.509 certificate authentication.
	// The [Mongodb.TLS] field must be configured when this mechanism is selected.
	MongoAuthMechanismTypeX509 MongoAuthMechanismType = "MONGODB-X509"

	// MongoAuthMechanismTypeSCRAMSHA256 uses SCRAM-SHA-256 (default, recommended).
	MongoAuthMechanismTypeSCRAMSHA256 MongoAuthMechanismType = "SCRAM-SHA-256"

	// MongoAuthMechanismTypeSCRAMSHA1 uses the legacy SCRAM-SHA-1 mechanism.
	MongoAuthMechanismTypeSCRAMSHA1 MongoAuthMechanismType = "SCRAM-SHA-1"
)

// String returns the string representation of the authentication mechanism type.
func (mt MongoAuthMechanismType) String() string {
	return string(mt)
}

// MongoPLAINCredentials represents the credentials for the PLAIN authentication mechanism.
// PLAIN authentication sends credentials as plain text and should only be used
// over secure connections (TLS).
type MongoPLAINCredentials struct {
	Username string `yaml:"username"`
	Password Secret `yaml:"password"`
}

// Validate validates the credentials for the PLAIN authentication mechanism.
// It ensures both username and password are provided.
//
// Returns an error if any validation rules fail.
func (m *MongoPLAINCredentials) Validate() error {
	return ValidateStruct(m,
		validation.Field(&m.Username, validation.Required),
		validation.Field(&m.Password, validation.Required),
	)
}

// MongoSCRAMCredentials represents the credentials for the SCRAM authentication mechanism.
// SCRAM (Salted Challenge Response Authentication Mechanism) is the default and
// recommended authentication mechanism for MongoDB.
type MongoSCRAMCredentials struct {
	Username   string `yaml:"username"`
	Password   Secret `yaml:"password"`
	AuthSource string `yaml:"authSource" default:"admin"`
}

// Validate validates the credentials for the SCRAM authentication mechanism.
// It ensures username, password, and authentication source are provided.
//
// Returns an error if any validation rules fail.
func (m *MongoSCRAMCredentials) Validate() error {
	return ValidateStruct(m,
		validation.Field(&m.Username, validation.Required),
		validation.Field(&m.Password, validation.Required),
		validation.Field(&m.AuthSource, validation.Required),
	)
}

// MongodbCredentials represents the credentials for the MongoDB connection.
// It contains the authentication mechanism type and corresponding credential details.
// Only the credentials matching the specified mechanism are required.
type MongodbCredentials struct {
	AuthMechanism MongoAuthMechanismType `yaml:"authMechanism" default:"SCRAM-SHA-256"`
	Plain         *MongoPLAINCredentials `yaml:"plain" default:"-"`
	Scram         *MongoSCRAMCredentials `yaml:"scram" default:"-"`
}

// Validate validates the credentials for the MongoDB connection.
// It ensures the authentication mechanism is valid and that the corresponding
// credential fields are properly configured.
//
// Returns an error if any validation rules fail.
func (m *MongodbCredentials) Validate() error {
	return ValidateStruct(m,
		validation.Field(&m.AuthMechanism, ozzo_rules.OneOf(
			MongoAuthMechanismTypeSCRAMSHA1,
			MongoAuthMechanismTypeSCRAMSHA256,
			MongoAuthMechanismTypeX509,
			MongoAuthMechanismTypePLAIN)),
		validation.Field(&m.Plain, validation.Required.When(m.AuthMechanism == MongoAuthMechanismTypePLAIN)),
		validation.Field(&m.Scram, validation.Required.When(m.AuthMechanism == MongoAuthMechanismTypeSCRAMSHA1 ||
			m.AuthMechanism == MongoAuthMechanismTypeSCRAMSHA256)),
	)
}

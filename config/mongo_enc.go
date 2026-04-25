// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// MongoKMSProvider represents the type of Key Management Service provider
// for MongoDB client-side field level encryption (CSFLE).
type MongoKMSProvider string

const (
	// MongoKMSProviderLocal is the local key provider. The local key service can be any component that assigns the key to
	// it. It can be integrated with local key vaults or API fetch using stores like Hashicrop and HSM.
	MongoKMSProviderLocal MongoKMSProvider = "local"

	// MongoKMSProviderAzure is the Azure Key Vault provider.
	MongoKMSProviderAzure MongoKMSProvider = "azure"

	// MongoKMSProviderAmazon is the AWS Key Management Service provider.
	MongoKMSProviderAmazon MongoKMSProvider = "amazon"

	// MongoKMSProviderGoogle is the Google Cloud KMS provider.
	MongoKMSProviderGoogle MongoKMSProvider = "google"
)

// MasterKeyLength is the required length in bytes for local master keys.
// Local master keys must be exactly 96 bytes when base64 encoded.
const MasterKeyLength = 96

// MongoKMS represents the Key Management Service configuration for MongoDB encryption.
// It contains provider-specific settings for managing encryption keys.
// Only the configuration matching the specified provider is required.
type MongoKMS struct {
	Provider MongoKMSProvider `yaml:"provider"`
	Local    *MongoKMSLocal   `yaml:"local" default:"-"`
	Amazon   *MongoKMSAmazon  `yaml:"amazon" default:"-"`
	Azure    *MongoKMSAzure   `yaml:"azure" default:"-"`
	Google   *MongoKMSGoogle  `yaml:"google" default:"-"`
}

// Validate validates the KMS configuration.
// It ensures the provider type is valid and that the corresponding
// provider-specific configuration is properly set.
//
// Returns an error if any validation rules fail.
func (m *MongoKMS) Validate() error {
	return ValidateStruct(m,
		validation.Field(&m.Provider, validation.Required,
			ozzo_rules.OneOf(MongoKMSProviderLocal, MongoKMSProviderAzure, MongoKMSProviderAmazon, MongoKMSProviderGoogle)),
		validation.Field(&m.Local, validation.Required.When(m.Provider == MongoKMSProviderLocal)),
		validation.Field(&m.Amazon, validation.Required.When(m.Provider == MongoKMSProviderAmazon)),
		validation.Field(&m.Azure, validation.Required.When(m.Provider == MongoKMSProviderAzure)),
		validation.Field(&m.Google, validation.Required.When(m.Provider == MongoKMSProviderGoogle)),
	)
}

// MongoKMSLocal represents the local Key Management Service provider.
// Local KMS uses locally managed keys and does not require external services.
// Not recommended for production environments due to security considerations.
type MongoKMSLocal struct {
	// A 96-byte long base64-encoded string. Locally managed keys do not require additional setup,
	// but are not recommended for production applications.
	MasterKey     Secret `yaml:"masterKey"`
	MasterKeyFile string `yaml:"masterKeyFile"`
}

// Validate validates the local KMS provider options.
// It ensures either a master key or master key file is provided,
// and validates the master key length if specified directly.
//
// Returns an error if any validation rules fail.
func (m *MongoKMSLocal) Validate() error {
	return ValidateStruct(m,
		validation.Field(&m.MasterKey,
			validation.Required.When(len(m.MasterKeyFile) == 0).Error("masterKey or masterKeyFile must be set"),
			validation.When(len(m.MasterKey) > 0, validation.Length(MasterKeyLength, MasterKeyLength))),
		validation.Field(&m.MasterKeyFile,
			validation.Required.When(len(m.MasterKey) == 0).Error("masterKey or masterKeyFile must be set"),
			ozzo_rules.File().When(len(m.MasterKeyFile) > 0)),
	)
}

// MongoKMSAmazon represents the AWS Key Management Service provider configuration.
// It contains all necessary credentials and settings for using AWS KMS
// for MongoDB client-side field level encryption.
type MongoKMSAmazon struct {
	AccessKeyId     string     `yaml:"accessKeyId"`
	SecretAccessKey Secret     `yaml:"secretAccessKey"`
	SessionToken    *string    `yaml:"sessionToken"`
	Key             string     `yaml:"key"`
	Region          *string    `yaml:"region"`
	Endpoint        *string    `yaml:"endpoint"`
	TLS             *TlsClient `yaml:"tls" default:"-"`
}

func validateRequiredAndOptional(structPtr any, requiredFields []any, optionalFields []any) error {
	fields := make([]*validation.FieldRules, 0, len(requiredFields)+len(optionalFields))
	for _, f := range requiredFields {
		fields = append(fields, validation.Field(f, validation.Required))
	}
	for _, f := range optionalFields {
		fields = append(fields, validation.Field(f, validation.NilOrNotEmpty))
	}
	return ValidateStruct(structPtr, fields...)
}

// Validate validates the AWS KMS provider options.
// It ensures all required AWS credentials and key information are provided.
//
// Returns an error if any validation rules fail.
func (m *MongoKMSAmazon) Validate() error {
	return validateRequiredAndOptional(m,
		[]any{&m.AccessKeyId, &m.SecretAccessKey, &m.Key},
		[]any{&m.SessionToken, &m.Region, &m.Endpoint, &m.TLS},
	)
}

// MongoKMSAzure represents the Azure Key Vault provider configuration.
// It contains all necessary credentials and settings for using Azure Key Vault
// for MongoDB client-side field level encryption.
type MongoKMSAzure struct {
	ClientId         string     `yaml:"clientId"`
	ClientSecret     Secret     `yaml:"clientSecret"`
	TenantId         string     `yaml:"tenantId"`
	KeyName          string     `yaml:"keyName"`
	KeyVersion       *string    `yaml:"keyVersion"`
	KeyVaultEndpoint *string    `yaml:"keyVaultEndpoint"`
	TLS              *TlsClient `yaml:"tls" default:"-"`
}

// Validate validates the Azure KMS provider options.
// It ensures all required Azure credentials and key vault information are provided.
//
// Returns an error if any validation rules fail.
func (m *MongoKMSAzure) Validate() error {
	return validateRequiredAndOptional(m,
		[]any{&m.ClientId, &m.ClientSecret, &m.TenantId, &m.KeyName, &m.KeyVaultEndpoint},
		[]any{&m.KeyVersion, &m.TLS},
	)
}

// MongoKMSGoogle represents the Google Cloud KMS provider configuration.
// It contains all necessary credentials and settings for using Google Cloud KMS
// for MongoDB client-side field level encryption.
type MongoKMSGoogle struct {
	Endpoint               *string    `yaml:"endpoint"`
	ProjectId              string     `yaml:"projectId"`
	Email                  string     `yaml:"email"`
	AuthenticationEndpoint *string    `yaml:"authenticationEndpoint"`
	PrivateKey             Secret     `yaml:"privateKey"`
	Location               string     `yaml:"location"`
	KeyRing                string     `yaml:"keyRing"`
	KeyName                string     `yaml:"keyName"`
	KeyVersion             *string    `yaml:"keyVersion"`
	TLS                    *TlsClient `yaml:"tls" default:"-"`
}

// Validate validates the GCP KMS provider options.
// It ensures all required Google Cloud credentials and key information are provided.
//
// Returns an error if any validation rules fail.
func (m *MongoKMSGoogle) Validate() error {
	return ValidateStruct(m,
		validation.Field(&m.Endpoint, validation.NilOrNotEmpty),
		validation.Field(&m.ProjectId, validation.Required),
		validation.Field(&m.Email, validation.Required),
		validation.Field(&m.AuthenticationEndpoint, validation.NilOrNotEmpty),
		validation.Field(&m.PrivateKey, validation.Required),
		validation.Field(&m.Location, validation.Required),
		validation.Field(&m.KeyRing, validation.Required),
		validation.Field(&m.KeyName, validation.Required),
		validation.Field(&m.KeyVersion, validation.NilOrNotEmpty),
		validation.Field(&m.TLS, validation.NilOrNotEmpty),
	)
}

// MongoEncryptionType represents the type of client-side field level encryption.
// MongoDB supports both automatic and manual encryption modes.
type MongoEncryptionType string

const (
	// MongoEncryptionTypeManual requires the application to explicitly encrypt and
	// decrypt each field using the encryption client API.
	MongoEncryptionTypeManual MongoEncryptionType = "manual"

	// MongoEncryptionTypeAuto lets the MongoDB driver transparently encrypt and
	// decrypt fields based on a JSON schema or encryption rules.
	MongoEncryptionTypeAuto MongoEncryptionType = "auto"
)

// MongoEncryption represents the client-side field level encryption configuration for MongoDB.
// It contains settings for the key vault, KMS provider, and encryption mode.
// Client-side encryption provides an additional layer of security by encrypting
// sensitive data before it leaves the application.
type MongoEncryption struct {
	VaultDatabase   *string             `yaml:"vaultDatabase"`
	VaultCollection string              `yaml:"vaultCollection" default:"__keyVault"`
	KMS             *MongoKMS           `yaml:"kms"`
	Type            MongoEncryptionType `yaml:"type" default:"auto"`
}

// Validate validates the MongoDB encryption configuration.
// It ensures the vault collection name, KMS configuration, and encryption type are valid.
//
// Returns an error if any validation rules fail.
func (m *MongoEncryption) Validate() error {
	return ValidateStruct(m,
		validation.Field(&m.VaultDatabase, validation.NilOrNotEmpty),
		validation.Field(&m.VaultCollection, validation.Required),
		validation.Field(&m.KMS, validation.Required),
		validation.Field(&m.Type, ozzo_rules.OneOf(MongoEncryptionTypeManual, MongoEncryptionTypeAuto)),
	)
}

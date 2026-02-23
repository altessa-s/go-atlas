// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=config --all-fields

import (
	"log/slog"
	"reflect"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"

	"github.com/altessa-s/go-atlas/data/mongo/kms"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Default tag names for field mapping
const (
	// DefaultBSONTagName is the default tag name used for BSON field mapping
	DefaultBSONTagName = "bson"
	// DefaultEncryptionTagName is the default tag name used for encryption field mapping
	DefaultEncryptionTagName = "encryption"
)

// Timeout Constants for various MongoDB operations
const (
	// DefaultCloseTimeout is the default timeout for closing MongoDB connections
	DefaultCloseTimeout = 30 * time.Second
	// DefaultPingTimeout is the default timeout for ping operations during connection
	DefaultPingTimeout = 5 * time.Second
)

// Default configuration constants
const (
	// DefaultVaultCollection is the default collection name for key vault
	DefaultVaultCollection = "__keyVault"
)

// EncryptionField defines how a specific struct field should be encrypted.
type EncryptionField struct {
	// FieldName is the name of the struct field to encrypt (case-sensitive)
	FieldName string
	// Algorithm is the encryption algorithm: "deterministic" or "random"
	Algorithm string
	// KeyAltName is the alternative name of the data key to use for encryption
	KeyAltName string
}

// EncryptionModel defines encryption configuration for a specific struct type.
type EncryptionModel struct {
	Type any
	// Fields defines which fields should be encrypted and how
	Fields []EncryptionField
}

// TransactionOptions defines configuration for MongoDB transactions.
type TransactionOptions struct {
	// ReadConcern specifies the read concern for the transaction
	ReadConcern *readconcern.ReadConcern
	// WriteConcern specifies the write concern for the transaction
	WriteConcern *writeconcern.WriteConcern
	// ReadPreference specifies the read preference for the transaction
	ReadPreference *readpref.ReadPref
	// MaxCommitTime specifies the maximum time for committing the transaction
	MaxCommitTime *time.Duration
}

// config holds all configuration for a MongoDB client instance.
type config struct {
	// KMS is the KMS provider for Client-Side Field Level Encryption
	KMS kms.Provider

	// VaultDatabase sets the database where encryption keys are stored.
	// If not set, uses the same database as the main connection.
	VaultDatabase string

	// VaultCollection sets the collection name for storing encryption keys.
	// Default is "__keyVault".
	VaultCollection string `optgen:"default=DefaultVaultCollection"`

	// EncryptionModels holds encryption configuration for struct types.
	// Note: This field uses custom handling in WithEncryptionModel.
	EncryptionModels map[reflect.Type]*EncryptionModel `opt:"-"`

	// EncryptionEnabled enables or disables Client-Side Field Level Encryption.
	EncryptionEnabled bool

	// BSONTagName sets the tag name used for BSON field mapping.
	// Default is "bson".
	BSONTagName string `optgen:"default=DefaultBSONTagName"`

	// EncryptionTagName sets the tag name used for encryption field mapping.
	// Default is "encryption".
	EncryptionTagName string `optgen:"default=DefaultEncryptionTagName"`

	// Client is a pre-initialized MongoDB client. When set, Connect() uses it
	// directly instead of creating a new one from ClientOptions.
	// Mutually exclusive with ClientOptions.
	Client *mongo.Client

	// ClientOptions allows setting custom mongo.ClientOptions for advanced configuration.
	ClientOptions *mongoOptions.ClientOptions

	// TransactionOptions holds transaction configuration settings.
	// Note: This field uses custom handling in WithTransactionOptions.
	TransactionOptions TransactionOptions `opt:"-" optgen:"default=DefaultTransactionOptions()"`

	// Logger sets a custom structured logger for the MongoDB client.
	Logger *slog.Logger
}

// WithTransactionOptions sets custom transaction options for MongoDB transactions.
// This allows fine-grained control over transaction behavior including read/write concerns
// and read preferences.
//
// Parameters:
//   - opts: Transaction options to apply. Only non-nil values will be used.
//
// Note: This uses custom handling because it merges values rather than replacing them.
func WithTransactionOptions(txOpts TransactionOptions) Option {
	return func(c *config) {
		if txOpts.ReadConcern != nil {
			c.TransactionOptions.ReadConcern = txOpts.ReadConcern
		}
		if txOpts.WriteConcern != nil {
			c.TransactionOptions.WriteConcern = txOpts.WriteConcern
		}
		if txOpts.ReadPreference != nil {
			c.TransactionOptions.ReadPreference = txOpts.ReadPreference
		}
	}
}

// WithEncryptionModel configures field-level encryption for a specific struct type.
// This allows specifying encryption configuration programmatically instead of using struct tags.
// The modelType parameter should be passed as (*YourStruct)(nil) to get the type.
//
// Parameters:
//   - model: The encryption model defining which fields to encrypt and how
//
// Note: This uses custom handling because it updates a map rather than replacing it.
func WithEncryptionModel(model ...*EncryptionModel) Option {
	return func(c *config) {
		if c.EncryptionModels == nil {
			c.EncryptionModels = make(map[reflect.Type]*EncryptionModel)
		}

		for _, m := range model {
			if m == nil || m.Type == nil {
				continue
			}
			typ := reflect.TypeOf(m.Type)
			if typ.Kind() == reflect.Pointer {
				typ = typ.Elem()
			}
			c.EncryptionModels[typ] = m
		}
	}
}

// DefaultTransactionOptions returns the default transaction options used by MongoDB transactions.
// This provides sensible defaults following MongoDB best practices for most use cases.
//
// Returns:
//   - TransactionOptions: Default configuration with Snapshot read concern, Majority write concern
func DefaultTransactionOptions() TransactionOptions {
	return TransactionOptions{
		ReadConcern:    readconcern.Snapshot(),
		WriteConcern:   writeconcern.Majority(),
		ReadPreference: nil, // Use default
		MaxCommitTime:  nil, // Not supported in MongoDB Go driver v2
	}
}

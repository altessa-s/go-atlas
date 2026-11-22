// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

//go:generate go tool optgen generate --type=config --all-fields
//go:generate go tool optgen generate --type=listOptions --option-type=ListOption --skip-option-type --skip-default-func --skip-new-func --option-prefix=List --output=list_options_gen.go
//go:generate go tool optgen generate --type=listCursorOptions --option-type=ListCursorOption --skip-option-type --skip-default-func --skip-new-func --option-prefix=ListCursor --output=list_cursor_options_gen.go

import (
	"context"
	"log/slog"
	"reflect"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/store/mongo/internal/document"
	"github.com/altessa-s/go-atlas/store/mongo/internal/query"
	"github.com/altessa-s/go-atlas/store/mongo/kms"

	converter "github.com/altessa-s/go-convert"
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
	// DefaultCursorCloseTimeout bounds best-effort cursor cleanup after a query.
	DefaultCursorCloseTimeout = query.CloseTimeout
)

// Default configuration constants
const (
	// DefaultVaultCollection is the default collection name for key vault
	DefaultVaultCollection = "__keyVault"
)

// DeduplicationIdentityFunc derives a caller-identity segment (typically
// tenant/subject) from the request context. It is mixed into the singleflight
// deduplication key for GetEntity/GetEntities so that two callers issuing the
// same filter concurrently are only collapsed into one in-flight query when they
// share an identity. Without it, an identical filter from different callers
// collapses and one may receive the other's result (a cross-tenant leak when
// isolation is enforced outside the filter, e.g. via CSFLE keys or read scope).
// Returning "" disables prefixing for that call, which is the default
// when no identity func is configured.
type DeduplicationIdentityFunc func(context.Context) string

// EncryptionField describes encryption for a struct field.
type EncryptionField = document.EncryptionField

// EncryptionModel describes encryption for a struct type.
type EncryptionModel = document.EncryptionModel

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

	// WithEncryptionModel validates models and merges them into the map.
	EncryptionModels map[reflect.Type]*EncryptionModel `optgen:"manual"`

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
	// WithTransactionOptions merges only non-nil supported fields.
	TransactionOptions TransactionOptions `optgen:"manual,default=DefaultTransactionOptions()"`

	// Logger sets a custom structured logger for the MongoDB client.
	Logger *slog.Logger

	// RollbackMigrationsOnFailure makes a failed migration run roll back every
	// applied migration (migrate.Down to the beginning). Off by default: a full
	// rollback drops the whole schema — idempotency indexes included — because
	// of one failing step, which is rarely what a running service wants.
	RollbackMigrationsOnFailure bool

	// Meter provides Prometheus-compatible metrics for MongoDB operations.
	Meter metrics.Meter `optgen:"notnil"`
	// WithConverterOptions adds converter.Option values that are appended to the
	// defaults when GetEntity and GetEntities convert decoded Mongo models to
	// domain entities. The built-in WithEmbeddedStructs(EmbeddedFillAlways) is always
	// applied first; user options follow and can extend the conversion pipeline.
	//
	// A typical use case is registering a codec that bridges custom field types,
	// for example optional.Optional[T] ↔ *T:
	//
	//	mongo.WithConverterOptions(converter.WithCodecs(optionalcodec.Codec))
	//
	// Successive calls accumulate options in registration order.
	ConverterOptions []converter.Option `opt:"ConverterOptions" optgen:"append"`
	// WithDeduplicationIdentity sets a function that derives a per-context identity
	// segment (typically tenant/subject) mixed into the GetEntity/GetEntities
	// singleflight deduplication key. This prevents two callers with the same filter
	// but different identities from collapsing onto one another's in-flight query.
	//
	// Returning "" from fn disables prefixing for that call, so existing callers are
	// unaffected when no identity func is configured.
	//
	//	m, _ := mongo.New("app", mongo.WithDeduplicationIdentity(func(ctx context.Context) string {
	//		p, _ := principal.FromContext(ctx) // zero Principal when unauthenticated
	//		return p.Tenant
	//	}))
	dedupIdentity DeduplicationIdentityFunc `opt:"DeduplicationIdentity"`
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
// Set each model's Type to (*YourStruct)(nil) or a struct value, and use the
// declared Go field names. Register embedded fields on their declaring type.
func WithEncryptionModel(model ...*EncryptionModel) Option {
	return func(c *config) {
		if c.EncryptionModels == nil {
			c.EncryptionModels = make(map[reflect.Type]*EncryptionModel)
		}

		for _, m := range model {
			if m == nil || m.Type == nil {
				// Preserve invalid declarations for New's validation.
				c.EncryptionModels[nil] = m
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

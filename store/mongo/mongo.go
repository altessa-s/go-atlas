// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"cmp"
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas-core/runtime/singleflight"
	"github.com/altessa-s/go-atlas/store/mongo/internal/query"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongodocument "github.com/altessa-s/go-atlas/store/mongo/internal/document"
	converter "github.com/altessa-s/go-convert"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Mongo represents a MongoDB client wrapper.
// It provides a high-level interface for MongoDB operations including:
//   - Connection management with automatic retries
//   - Transaction handling with configurable retry logic
//   - Client-Side Field Level Encryption (CSFLE) using explicit encryption
//   - Database migrations
//   - Structured logging integration
type Mongo struct {
	conn       connection
	repo       repository
	encryption fieldEncryption
	schema     schemaSetup

	// VersionString is the version string of the MongoDB server (e.g., "5.0.8")
	VersionString string

	// VersionMajor is the major version of the MongoDB server
	VersionMajor int32

	// VersionMinor is the minor version of the MongoDB server
	VersionMinor int32
}

// New creates a new MongoDB client instance using option functions.
// This provides a flexible and extensible API without external dependencies.
//
// Example:
//
//	// Basic usage with minimal configuration
//	mongo, err := mongotools.New("myapp",
//		mongotools.WithLogger(logger),
//	)
//
//	// Advanced usage with custom client options
//	clientOpts := options.Client().ApplyURI("mongodb://localhost:27017").
//		SetAuth(options.Credential{Username: "user", Password: "pass"})
//
//	mongo, err := mongotools.New("myapp",
//		mongotools.WithClientOptions(clientOpts),
//		mongotools.WithLogger(logger),
//	)
func New(database string, opts ...Option) (*Mongo, error) {
	// Create config with defaults and apply provided options
	cfg := newConfig(opts...)

	if cfg.Client != nil && cfg.ClientOptions != nil {
		return nil, errors.New("WithClient and WithClientOptions are mutually exclusive")
	}

	var registry *bson.Registry
	if cfg.ClientOptions != nil {
		registry = cfg.ClientOptions.Registry
	}
	if registry == nil {
		registry = query.ClientRegistry(cfg.Client)
	}
	if cfg.Client == nil {
		cfg.ClientOptions = mongoOptions.MergeClientOptions(mongoOptions.Client().SetRegistry(registry), cfg.ClientOptions)
	}
	metrics := newMongoMetrics(cfg.Meter)
	m := &Mongo{
		conn: connection{
			database:           database,
			externalClient:     cfg.Client,
			clientOptions:      cfg.ClientOptions,
			transactionOptions: cfg.TransactionOptions,
			metrics:            metrics,
			logger:             cfg.Logger,
		},
		repo: repository{
			metrics:       metrics,
			dedupIdentity: cfg.dedupIdentity,
			converterOpts: append([]converter.Option{converter.WithEmbeddedStructs(converter.EmbeddedFillAlways)}, cfg.ConverterOptions...),
		},
		encryption: fieldEncryption{registry: registry},
		schema:     schemaSetup{registry: cfg.migrationRegistry, rollbackOnFailure: cfg.RollbackMigrationsOnFailure, logger: cfg.Logger},
	}
	if cfg.readDeduplication {
		m.repo.singleFlight = singleflight.New[readFlightKey, []bson.Raw]()
	}

	encryptedFields, err := mongodocument.New(cfg.EncryptionTagName, cfg.EncryptionModels, m.encryption.encryptDocumentField)
	if err != nil {
		return nil, err
	}
	m.repo.initTranslators(registry, cfg.BSONTagName, encryptedFields)

	// Setup encryption if the KMS provider is configured and encryption is enabled
	if cfg.EncryptionEnabled {
		if cfg.KMS == nil {
			return nil, errors.New("KMS provider is required for encryption - use WithKMS() option")
		}

		m.encryption.provider = cfg.KMS
		m.encryption.vaultDatabase = cmp.Or(cfg.VaultDatabase, database)
		m.encryption.vaultCollection = cfg.VaultCollection
	}

	return m, nil
}

// Connect establishes a connection to the MongoDB server.
//
// The connection process includes:
//   - Establishing the main MongoDB connection with timeout
//   - Pinging the server to verify connectivity
//   - Retrieving and validating server version (minimum 5.0 required)
//   - Running database migrations with separate timeout
//
// Parameters:
//   - ctx: Context for controlling connection timeout and cancellation
//
// Returns:
//   - error: Error if any step of the connection process fails
//
// A failed connection attempt cleans up the resources it opened.
func (m *Mongo) Connect(ctx context.Context) (err error) {
	m.conn.connectMu.Lock()
	defer m.conn.connectMu.Unlock()

	if m.conn.closing.Load() != 0 {
		return mongo.ErrClientDisconnected
	}
	if m.conn.connected {
		return nil
	}

	stop := m.conn.metrics.connectDuration.Start()
	defer stop()
	defer func() {
		if err != nil {
			m.closeLocked(ctx)
		}
	}()

	if err = m.conn.open(ctx); err != nil {
		return err
	}

	// Get mongo info: version and modules.
	if err = m.getServerInfo(ctx); err != nil {
		return
	}

	if m.VersionMajor < MinRequiredMongoMajorVersion {
		return coreerrs.Wrapf(ErrUnsupportedVersion, "the minimum required version is %d.0 but "+
			"current version is %s", MinRequiredMongoMajorVersion, m.VersionString)
	}

	if err = m.encryption.open(ctx, m.Client()); err != nil {
		return err
	}
	if err = m.schema.migrate(ctx, m.Database()); err != nil {
		return err
	}
	if m.conn.closing.Load() != 0 {
		return mongo.ErrClientDisconnected
	}
	m.conn.connected = true
	return nil
}

// DatabaseName returns the name of the MongoDB database being used.
// This is the database name specified in the configuration.
//
// Returns:
//   - string: The configured database name
func (m *Mongo) DatabaseName() string {
	return m.conn.database
}

// Database returns the [mongo.Database] handle for the configured database name.
func (m *Mongo) Database() *mongo.Database {
	return m.Client().Database(m.conn.database)
}

// Client returns the MongoDB client for database operations.
// This client is used for all operations including regular database
// operations and encryption key vault access.
//
// Returns:
//   - *mongo.Client: The MongoDB client instance to use for operations
func (m *Mongo) Client() *mongo.Client {
	return m.conn.client.Load()
}

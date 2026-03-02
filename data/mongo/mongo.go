// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/mongo/kms"
	"github.com/altessa-s/go-atlas/domain/converter"

	"golang.org/x/sync/singleflight"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/runtime/retry"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Ensure kms package is recognized as used
var _ kms.Provider

// Connection configuration constants
const (
	// MinRequiredMongoMajorVersion is the minimum required MongoDB major version
	MinRequiredMongoMajorVersion = 5
	// DefaultPingMaxRetries is the default number of retry attempts for ping operations
	DefaultPingMaxRetries = 3
	// DefaultPingBaseDelay is the default base delay between ping retry attempts
	DefaultPingBaseDelay = 100 * time.Millisecond
)

// Server info command constants
const (
	// BuildInfoCommand is the MongoDB command to retrieve server build information
	BuildInfoCommand = "buildInfo"
	// VersionFieldName is the field name for version string in buildInfo response
	VersionFieldName = "version"
	// VersionArrayFieldName is the field name for version array in buildInfo response
	VersionArrayFieldName = "versionArray"
	// MinVersionArrayLength is the minimum required length for version array parsing
	MinVersionArrayLength = 2
)

// Mongo represents a MongoDB client wrapper.
// It provides a high-level interface for MongoDB operations including:
//   - Connection management with automatic retries
//   - Transaction handling with configurable retry logic
//   - Client-Side Field Level Encryption (CSFLE) using explicit encryption
//   - Database migrations
//   - Structured logging integration
type Mongo struct {
	// options holds all configuration for the MongoDB client
	config *config

	database string

	// client is the MongoDB client instance used for all operations including encryption
	client *mongo.Client

	// encryptionClient handles Client-Side Field Level Encryption operations
	encryptionClient *mongo.ClientEncryption

	// vaultNamespace is the full namespace for the key vault (database.collection)
	vaultNamespace string

	// dkids is a thread-safe map that caches data key IDs by alt name
	dkids sync.Map

	// VersionString is the version string of the MongoDB server (e.g., "5.0.8")
	VersionString string

	// VersionMajor is the major version of the MongoDB server
	VersionMajor int32

	// VersionMinor is the minor version of the MongoDB server
	VersionMinor int32

	// ownsClient is true when Connect() created the client internally.
	// When false, Close() will not call Disconnect on the client.
	ownsClient bool

	// connected is an atomic boolean indicating the connection state
	connected atomic.Bool
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
//		mongotools.WithMongoClientOptions(clientOpts),
//		mongotools.WithLogger(logger),
//	)
func New(database string, opts ...Option) (*Mongo, error) {
	// Create config with defaults and apply provided options
	cfg := newConfig(opts...)

	if cfg.Client != nil && cfg.ClientOptions != nil {
		return nil, errors.New("WithClient and WithClientOptions are mutually exclusive")
	}

	// Create a Mongo instance with config
	m := &Mongo{
		config:   cfg,
		database: database,
	}

	// Setup encryption if the KMS provider is configured and encryption is enabled
	if m.config.EncryptionEnabled {
		if cfg.KMS == nil {
			return nil, errors.New("KMS provider is required for encryption - use WithKMS() option")
		}

		// Use the main database for vault if not specified
		m.config.VaultDatabase = cmp.Or(m.config.VaultDatabase, m.database)

		m.vaultNamespace = m.config.VaultDatabase + "." + m.config.VaultCollection
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
// Note: If an error occurs, the connection is automatically cleaned up by calling Close().
func (m *Mongo) Connect(ctx context.Context) (err error) {
	if m.connected.Load() {
		return nil
	}

	defer func() { //nolint:contextcheck // Close() creates its own context internally
		if err == nil {
			m.connected.Store(true)
			return
		}

		m.Close()
	}()

	if m.config.Client != nil {
		m.client = m.config.Client
	} else {
		opts := cmp.Or(m.config.ClientOptions, mongoOptions.Client())

		m.client, err = mongo.Connect(opts)
		if err != nil {
			return
		}

		m.ownsClient = true
	}

	if err = m.ping(ctx, m.client); err != nil {
		return
	}

	// Get mongo info: version and modules.
	if err = m.getServerInfo(ctx); err != nil {
		return
	}

	if m.VersionMajor < MinRequiredMongoMajorVersion {
		return coreerrs.Wrapf(ErrUnsupportedVersion, "the minimum required version is %d.0 but "+
			"current version is %s", MinRequiredMongoMajorVersion, m.VersionString)
	}

	// If encryption is enabled, we create encryption client.
	if m.IsEncryptionConfigured() {
		// Create collection for key vault. This is a one-time operation.
		// The key vault collection should be created before using encryption.
		if err = m.createKeyVaultCollection(ctx, m.config.VaultDatabase, m.config.VaultCollection); err != nil {
			return
		}

		clientEncryptionOpts := mongoOptions.ClientEncryption().
			SetKeyVaultNamespace(m.vaultNamespace).
			SetKmsProviders(m.config.KMS.Credentials())

		if tlsConfig := m.config.KMS.TLSConfig(); tlsConfig != nil {
			clientEncryptionOpts.SetTLSConfig(map[string]*tls.Config{
				m.config.KMS.Name(): tlsConfig,
			})
		}

		// Use the main client for encryption operations (MongoDB Go driver explicitly supports this)
		if m.encryptionClient, err = mongo.NewClientEncryption(m.client, clientEncryptionOpts); err != nil {
			return
		}
	}

	migrationCtx, migrationCtxCancel := corecontext.WithMaxTimeout(ctx, MigrationTimeout)
	defer migrationCtxCancel()

	err = m.migrate(migrationCtx)

	return
}

// WithTransaction executes a function within a MongoDB transaction using MongoDB's native
// retry logic. It uses the driver's built-in Session.WithTransaction method which automatically
// handles transient errors, commit retries, and transaction lifecycle management.
//
// The transaction uses configurable options from TransactionOptions including:
//   - Read concern, write concern, and read preference
//   - MongoDB's optimized retry logic for transient errors
//   - Automatic transaction lifecycle management
//
// Parameters:
//   - ctx: Context for controlling transaction timeout and cancellation
//   - handler: Function to execute within the transaction. It receives a session context
//     that must be used for all database operations within the transaction.
//
// Returns:
//   - error: Error if transaction fails or if handler panics
//
// Example:
//
//	err := mongo.WithTransaction(ctx, func(sessCtx context.Context) error {
//		// Use sessCtx for all operations within the transaction
//		_, err := collection.InsertOne(sessCtx, document)
//		return err
//	})
func (m *Mongo) WithTransaction(ctx context.Context, handler func(ctx context.Context) error) error {
	// Apply transaction timeout (cap to DefaultTxTimeout)
	ctxWithTimeout, cancel := corecontext.WithMaxTimeout(ctx, DefaultTxTimeout)
	defer cancel()

	startTime := time.Now()

	sess, err := m.Client().StartSession()
	if err != nil {
		return coreerrs.WrapOperation(err, "start session")
	}
	defer sess.EndSession(ctxWithTimeout)

	txnOpts := mongoOptions.Transaction()

	// Apply configurable transaction options
	if m.config.TransactionOptions.ReadConcern != nil {
		txnOpts.SetReadConcern(m.config.TransactionOptions.ReadConcern)
	}
	if m.config.TransactionOptions.WriteConcern != nil {
		txnOpts.SetWriteConcern(m.config.TransactionOptions.WriteConcern)
	}
	if m.config.TransactionOptions.ReadPreference != nil {
		txnOpts.SetReadPreference(m.config.TransactionOptions.ReadPreference)
	}

	var panicErr error
	_, err = sess.WithTransaction(ctxWithTimeout, func(sessCtx context.Context) (any, error) {
		// Handle panics within the transaction
		defer panics.HandleWithOpts(sessCtx, panics.NewHandleOpts().SetReallyPanic(false), func(_ context.Context, r any) {
			if m.config.Logger != nil {
				m.config.Logger.Error("transaction panicked", slog.Any("panic", r))
			}
			panicErr = fmt.Errorf("transaction panicked: %v", r)
		})

		if handlerErr := handler(sessCtx); handlerErr != nil {
			return nil, handlerErr
		}

		// Check if panic occurred during handler execution
		if panicErr != nil {
			return nil, panicErr
		}

		return struct{}{}, nil // Return empty struct instead of nil value
	}, txnOpts)

	// Panic errors take priority over regular errors
	if panicErr != nil {
		err = panicErr
	}

	if err != nil {
		if m.config.Logger != nil {
			m.config.Logger.Error("transaction failed", slog.Any("error", err),
				"duration_ms", time.Since(startTime).Milliseconds())
		}
		return err
	}

	if m.config.Logger != nil {
		m.config.Logger.Debug("transaction completed successfully",
			"duration_ms", time.Since(startTime).Milliseconds())
	}

	return nil
}

// Close gracefully closes all MongoDB connections and cleans up resources.
// It safely handles multiple calls and ensures the client is properly disconnected.
//
// The close process includes:
//   - Closing the encryption client (if configured)
//   - Disconnecting the main MongoDB client
//   - Logging the successful closure
//
// This method is safe to call multiple times and should be called when the
// MongoDB client is no longer needed, typically in a defer statement.
func (m *Mongo) Close() {
	if m.connected.CompareAndSwap(true, false) {
		if m.client == nil {
			return
		}

		defer func() {
			if m.config.Logger != nil {
				m.config.Logger.Info("mongo connection closed")
			}
		}()

		ctx, cancel := corecontext.ApplyTimeout(context.Background(), DefaultCloseTimeout)
		defer cancel()

		if m.encryptionClient != nil {
			if closeErr := m.encryptionClient.Close(ctx); closeErr != nil {
				if m.config.Logger != nil {
					m.config.Logger.Warn("failed to close encryption client", "error", closeErr)
				}
			}
		}

		if m.ownsClient {
			if disconnectErr := m.client.Disconnect(ctx); disconnectErr != nil && !errors.Is(disconnectErr, mongo.ErrClientDisconnected) {
				if m.config.Logger != nil {
					m.config.Logger.Warn("failed to disconnect mongo client", "error", disconnectErr)
				}
			}
		}
	}
}

// DatabaseName returns the name of the MongoDB database being used.
// This is the database name specified in the configuration.
//
// Returns:
//   - string: The configured database name
func (m *Mongo) DatabaseName() string {
	return m.database
}

// Database returns the [mongo.Database] handle for the configured database name.
func (m *Mongo) Database() *mongo.Database {
	return m.Client().Database(m.database)
}

// Client returns the MongoDB client for database operations.
// This client is used for all operations including regular database
// operations and encryption key vault access.
//
// Returns:
//   - *mongo.Client: The MongoDB client instance to use for operations
func (m *Mongo) Client() *mongo.Client {
	return m.client
}

var singleFlight *singleflight.Group

func init() {
	singleFlight = &singleflight.Group{}
}

// GetEntity retrieves a single entity from MongoDB by converting a document model to domain entity.
// It uses generics to provide type-safe conversion from MongoDB document type T to domain entity type E.
// The function includes request deduplication using singleflight to prevent duplicate concurrent queries.
//
// This function is designed to be used with a Mongo instance for proper singleflight deduplication.
// The Mongo instance must be fully initialized with a connected singleFlight group.
//
// Type parameters:
//   - T: MongoDB document model type
//   - E: Domain entity type to convert to
//
// Parameters:
//   - ctx: Context for controlling query timeout and cancellation
//   - m: Mongo client instance containing singleflight for deduplication
//   - col: MongoDB collection to query
//   - filter: BSON filter criteria for finding the document
//
// Returns:
//   - E: The converted domain entity
//   - error: mongo.ErrNoDocuments if not found, or other errors for decode/conversion failures
//
// Example:
//
//	type UserModel struct { ID string `bson:"_id"` }
//	type UserEntity struct { ID string }
//
//	user, err := mongotools.GetEntity[UserModel, UserEntity](ctx, mongo, collection, bson.M{"_id": "123"})
func GetEntity[T any, E any](ctx context.Context, col *mongo.Collection, filter bson.M) (E, error) {
	var zero E

	// Generate a deduplication key from filter for singleflight request deduplication
	// Include database name to prevent cross-database data leakage
	deduplicationKey := generateDeduplicationKey(col.Database().Name()+":"+col.Name(), filter)

	// Use singleflight to prevent duplicate concurrent requests
	data, err, _ := singleFlight.Do(deduplicationKey, func() (any, error) {
		// Apply timeout for database operation
		ctxWithTimeout, cancel := corecontext.WithMaxTimeout(ctx, DefaultQueryTimeout)
		defer cancel()

		// Find the document in MongoDB
		var model T
		result := col.FindOne(ctxWithTimeout, filter)
		if err := result.Decode(&model); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return zero, mongo.ErrNoDocuments
			}
			return zero, coreerrs.WrapOperation(err, "decode document")
		}

		var dst any

		entityType := reflect.TypeFor[E]()
		if entityType.Kind() == reflect.Pointer {
			// E is a pointer, create a new instance
			dst = reflect.New(entityType.Elem()).Interface()
		} else {
			// E is a struct, create a pointer to zero value
			dst = reflect.New(entityType).Interface()
		}

		converter.Convert(model, dst, converter.WithHandleEmbeddedStructs(true))

		// Return the correct type
		if entityType.Kind() == reflect.Pointer {
			return dst, nil
		}
		// If E is a struct, dereference the pointer
		return reflect.ValueOf(dst).Elem().Interface(), nil
	})

	if err != nil {
		return zero, err
	}

	entity, ok := data.(E)
	if !ok {
		return zero, fmt.Errorf("singleflight type assertion failed: got %T, expected %T", data, zero)
	}

	return entity, nil
}

// GetEntities retrieves multiple entities from MongoDB by converting document models to domain entities.
// It uses generics to provide type-safe conversion from MongoDB document type T to domain entity type E.
// The function includes request deduplication using singleflight to prevent duplicate concurrent queries.
//
// This function is designed to be used with a Mongo instance for proper singleflight deduplication.
// The Mongo instance must be fully initialized with a connected singleFlight group.
//
// Type parameters:
//   - T: MongoDB document model type
//   - E: Domain entity type to convert to
//
// Parameters:
//   - ctx: Context for controlling query timeout and cancellation
//   - m: Mongo client instance containing singleflight for deduplication
//   - col: MongoDB collection to query
//   - filter: BSON filter criteria for finding documents
//
// Returns:
//   - []E: Slice of converted domain entities (nil if no documents found)
//   - error: Error for query execution, decode, or conversion failures
//
// Example:
//
//	type UserModel struct { ID string `bson:"_id"` }
//	type UserEntity struct { ID string }
//
//	users, err := mongotools.GetEntities[UserModel, UserEntity](ctx, mongo, collection, bson.M{"active": true})
func GetEntities[T any, E any](ctx context.Context, col *mongo.Collection, filter bson.M) ([]E, error) {
	// Generate a deduplication key from filter for singleflight request deduplication
	// Include database name to prevent cross-database data leakage
	deduplicationKey := generateDeduplicationKey(col.Database().Name()+":"+col.Name()+deduplicationKeySuffixList, filter)

	// Use singleflight to prevent duplicate concurrent requests
	data, err, _ := singleFlight.Do(deduplicationKey, func() (any, error) {
		// Apply timeout for database operation
		ctxWithTimeout, cancel := corecontext.WithMaxTimeout(ctx, DefaultQueryTimeout)
		defer cancel()

		// Execute a query to find matching documents
		cursor, err := col.Find(ctxWithTimeout, filter)
		if err != nil {
			return []E{}, coreerrs.WrapOperation(err, "execute query")
		}
		defer func() {
			if cerr := cursor.Close(ctx); cerr != nil {
				// Log cursor close error but don't override main error
				// In production code, you might want to use a proper logger here
				_ = cerr // Explicitly ignore the error to satisfy linter
			}
		}()

		// Decode all matching documents
		var models []T
		if err = cursor.All(ctxWithTimeout, &models); err != nil {
			return []E{}, coreerrs.WrapOperation(err, "decode documents")
		}

		// Convert MongoDB documents to domain entities with pre-allocated slice
		entityType := reflect.TypeFor[E]()
		entities := make([]E, 0, len(models))

		for _, model := range models {
			var dst any
			if entityType.Kind() == reflect.Pointer {
				// E is a pointer, create a new instance
				dst = reflect.New(entityType.Elem()).Interface()
			} else {
				// E is a struct, create a pointer to zero value
				dst = reflect.New(entityType).Interface()
			}

			converter.Convert(model, dst, converter.WithHandleEmbeddedStructs(true))

			// Append the correct type to slice
			if entityType.Kind() == reflect.Pointer {
				entity, _ := dst.(E) //nolint:errcheck
				entities = append(entities, entity)
			} else {
				// If E is a struct, dereference the pointer
				entity, _ := reflect.ValueOf(dst).Elem().Interface().(E) //nolint:errcheck
				entities = append(entities, entity)
			}
		}

		return entities, nil
	})

	if err != nil {
		return nil, err
	}

	entities, ok := data.([]E)
	if !ok {
		return nil, fmt.Errorf("singleflight type assertion failed: got %T, expected []%T", data, *new(E))
	}

	return entities, nil
}

// createKeyVaultCollection creates the key vault collection for encryption keys.
func (m *Mongo) createKeyVaultCollection(ctx context.Context, database, collection string) error {
	// Create the key vault collection.
	_, err := m.client.Database(database).
		RunCommand(ctx, bson.D{{Key: "create", Value: collection}}).
		Raw()

	if err != nil {
		serr, ok := coreerrs.AsType[mongo.ServerError](err)
		if !ok || !serr.HasErrorCode(MongoErrorCodeCollectionExists) {
			return err
		}
	}

	keyVaultCollection := m.client.Database(database).Collection(collection)

	_, err = keyVaultCollection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "keyAltNames", Value: 1}},
		Options: mongoOptions.Index().
			SetUnique(true).
			SetPartialFilterExpression(bson.D{
				{Key: "keyAltNames", Value: bson.D{{Key: "$exists", Value: true}}},
			}),
	})

	return err
}

func (m *Mongo) ping(ctx context.Context, client *mongo.Client) error {
	return coreretry.Do(ctx, coreretry.Config{
		MaxAttempts: DefaultPingMaxRetries - 1, // 0-based: attempts 0..N-1 = N total calls
		ShouldRetry: IsTransientTransaction,
		NextDelay: func(attempt int, _ error) time.Duration {
			// Linear backoff: (attempt+1) * base delay
			return time.Duration(attempt+1) * DefaultPingBaseDelay
		},
		OnRetry: func(attempt int, err error, nextDelay time.Duration) {
			if m.config.Logger != nil {
				m.config.Logger.Debug("ping failed, retrying",
					slog.Int("attempt", attempt+1),
					slog.Int64("delay_ms", nextDelay.Milliseconds()),
					slog.Any("error", err))
			}
		},
	}, func(ctx context.Context) error {
		pingCtx, pingCtxCancel := corecontext.WithMaxTimeout(ctx, DefaultPingTimeout)
		defer pingCtxCancel()

		if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
			if !IsTransientTransaction(err) {
				return coreerrs.Wrap(err, "ping failed")
			}
			return err
		}
		return nil
	})
}

// getServerInfo retrieves MongoDB server version information using the buildInfo command.
// It populates the VersionString, VersionMajor, and VersionMinor fields of the Mongo instance.
// Returns an error if the command fails or if version information cannot be parsed.
func (m *Mongo) getServerInfo(ctx context.Context) error {
	buildInfo, err := m.executeBuildInfoCommand(ctx)
	if err != nil {
		return coreerrs.WrapOperation(err, "execute buildInfo command")
	}

	if err = m.parseVersionString(buildInfo); err != nil {
		if m.config.Logger != nil {
			m.config.Logger.Warn("failed to parse version string", "error", err)
		}
	}

	if err = m.parseVersionArray(buildInfo); err != nil {
		if m.config.Logger != nil {
			m.config.Logger.Warn("failed to parse version array", "error", err)
		}
	}

	// If we couldn't get version info at all, return error
	if m.VersionString == "" && m.VersionMajor == 0 {
		return errors.New("unable to determine MongoDB server version")
	}

	return nil
}

// executeBuildInfoCommand executes the MongoDB buildInfo command and returns the response.
func (m *Mongo) executeBuildInfoCommand(ctx context.Context) (bson.M, error) {
	var buildInfo bson.M

	cmd := bson.M{BuildInfoCommand: 1}
	opts := mongoOptions.RunCmd().SetReadPreference(readpref.Primary())

	err := m.Client().Database(m.database).RunCommand(ctx, cmd, opts).Decode(&buildInfo)
	if err != nil {
		return nil, err
	}

	return buildInfo, nil
}

// parseVersionString extracts and sets the version string from buildInfo response.
func (m *Mongo) parseVersionString(buildInfo bson.M) error {
	versionValue, exists := buildInfo[VersionFieldName]
	if !exists {
		return errors.New("version field not found in buildInfo response")
	}

	versionStr, ok := versionValue.(string)
	if !ok {
		return fmt.Errorf("version field is not a string, got %T", versionValue)
	}

	if versionStr == "" {
		return errors.New("version string is empty")
	}

	m.VersionString = versionStr
	return nil
}

// parseVersionArray extracts and sets major/minor version numbers from buildInfo response.
func (m *Mongo) parseVersionArray(buildInfo bson.M) error {
	versionArrayValue, exists := buildInfo[VersionArrayFieldName]
	if !exists {
		return errors.New("versionArray field not found in buildInfo response")
	}

	versionArray, ok := versionArrayValue.(bson.A)
	if !ok {
		return fmt.Errorf("versionArray field is not an array, got %T", versionArrayValue)
	}

	if len(versionArray) < MinVersionArrayLength {
		return fmt.Errorf("versionArray has insufficient elements: got %d, need at least %d",
			len(versionArray), MinVersionArrayLength)
	}

	major, ok := versionArray[0].(int32)
	if !ok {
		return fmt.Errorf("major version is not int32, got %T", versionArray[0])
	}

	minor, ok := versionArray[1].(int32)
	if !ok {
		return fmt.Errorf("minor version is not int32, got %T", versionArray[1])
	}

	m.VersionMajor = major
	m.VersionMinor = minor
	return nil
}

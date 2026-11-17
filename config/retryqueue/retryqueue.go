// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueueconfig

import (
	"time"

	"github.com/altessa-s/go-atlas/config/internal/validators"

	validationconfig "github.com/altessa-s/go-nomos/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// StorageType selects the backend that persists retry items.
type StorageType string

const (
	// StorageTypeMemory keeps items in process (single-node, tests).
	StorageTypeMemory StorageType = "memory"
	// StorageTypeMongo persists items in a MongoDB collection.
	StorageTypeMongo StorageType = "mongo"
	// StorageTypeSQL persists items in a SQL table (PostgreSQL, MySQL or
	// MariaDB) through an injected *sql.DB.
	StorageTypeSQL StorageType = "sqldb"
)

// SQL dialects accepted by [SQLStorageConfig.Dialect].
const (
	// SQLDialectPostgres targets PostgreSQL 12+.
	SQLDialectPostgres = "postgres"
	// SQLDialectMySQL targets MySQL 8.0+ and MariaDB 10.6+.
	SQLDialectMySQL = "mysql"
)

var storageAllowedTypes = []StorageType{StorageTypeMemory, StorageTypeMongo, StorageTypeSQL}

// MemoryStorageConfig configures the in-process retry queue store
// (reliability/retryqueue/storages/memory). The backend has no tunables; the
// type exists so every storage backend has a dedicated, discoverable config
// section.
type MemoryStorageConfig struct{}

// Validate performs validation of the in-memory storage configuration.
func (c *MemoryStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c)
}

// MongoStorageConfig configures the retry queue store backed by a MongoDB
// collection (reliability/retryqueue/storages/mongo).
type MongoStorageConfig struct {
	// Collection is the MongoDB collection that stores retry items.
	Collection string `yaml:"collection" default:"retry_items"`
	// FlagField is the document field of the attention flag, so the store can
	// adopt a collection whose documents name it differently.
	FlagField string `yaml:"flagField" default:"flagged"`
	// FlagReasonField is the document field of the flag's reason.
	FlagReasonField string `yaml:"flagReasonField" default:"flagReason"`
	// ResolvedRetention is how long a resolved item is kept before MongoDB's
	// TTL monitor purges it; zero keeps resolved items until they are deleted.
	ResolvedRetention time.Duration `yaml:"resolvedRetention"`
	// EnsureIndexes makes the factory create the collection's indexes while
	// building the queue, through the store's idempotent EnsureIndexes. Leave
	// it false when the indexes are managed elsewhere.
	EnsureIndexes bool `yaml:"ensureIndexes" default:"false"`
}

// Validate performs validation of the MongoDB storage configuration.
func (c *MongoStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Collection, validation.Required),
		validation.Field(&c.FlagField, validation.Required),
		validation.Field(&c.FlagReasonField, validation.Required),
		validation.Field(&c.ResolvedRetention, validation.When(c.ResolvedRetention != 0, validationconfig.PositiveDuration())),
	)
}

// SQLStorageConfig configures the retry queue store backed by a SQL table
// (reliability/retryqueue/storages/sqldb). The *sql.DB itself is injected into
// the factory; the caller chooses and registers the driver.
type SQLStorageConfig struct {
	// Dialect selects the SQL flavor: "postgres" or "mysql" (MySQL 8.0+ and
	// MariaDB 10.6+).
	Dialect string `yaml:"dialect" default:"postgres"`
	// Table is the table that stores retry items, optionally schema-qualified.
	Table string `yaml:"table" default:"retry_items"`
	// EnsureSchema makes the factory create the table and its indexes while
	// building the queue, through the store's idempotent EnsureSchema. Leave
	// it false when the schema is applied through migrations.
	EnsureSchema bool `yaml:"ensureSchema" default:"false"`
	// ResolvedRetention is how long the store's PurgeExpired keeps a
	// resolved item; zero keeps resolved items until they are deleted.
	ResolvedRetention time.Duration `yaml:"resolvedRetention"`
	// PurgeInterval is the time between background purge passes. Zero disables
	// the loop; explicit PurgeExpired calls remain available.
	PurgeInterval time.Duration `yaml:"purgeInterval" default:"1m"`
	// PurgeBatchSize bounds the rows deleted by one purge statement.
	PurgeBatchSize int `yaml:"purgeBatchSize" default:"1000"`
}

// Validate performs validation of the SQL storage configuration.
func (c *SQLStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Dialect, validation.Required, ozzo_rules.OneOf(SQLDialectPostgres, SQLDialectMySQL)),
		validation.Field(&c.Table, validation.Required, validators.SQLTableName),
		validation.Field(&c.ResolvedRetention, validation.When(c.ResolvedRetention != 0, validationconfig.PositiveDuration())),
		validation.Field(&c.PurgeInterval, validation.Min(time.Duration(0))),
		validation.Field(&c.PurgeBatchSize, validation.Min(1)),
	)
}

// StorageConfig selects and configures the retry queue backend. Each backend
// has its own nested section; only the one named by Type is used.
type StorageConfig struct {
	// Type defines the storage backend type (memory, mongo or sqldb).
	Type StorageType `yaml:"type" default:"memory"`

	// Memory defines the in-memory configuration.
	// Required when Type is StorageTypeMemory, ignored otherwise.
	Memory *MemoryStorageConfig `yaml:"memory" default:"-"`

	// Mongo defines the MongoDB configuration.
	// Required when Type is StorageTypeMongo, ignored otherwise.
	Mongo *MongoStorageConfig `yaml:"mongo" default:"-"`

	// SQL defines the SQL database configuration.
	// Required when Type is StorageTypeSQL, ignored otherwise.
	SQL *SQLStorageConfig `yaml:"sqldb" default:"-"`
}

// DefaultStorage returns the default storage configuration (in-memory).
func DefaultStorage() *StorageConfig {
	return &StorageConfig{
		Type:   StorageTypeMemory,
		Memory: &MemoryStorageConfig{},
	}
}

// Normalize allocates the backend-specific sub-config implied by
// [StorageConfig.Type]. Call Normalize before Validate so the correct
// sub-struct is present for validation.
func (c *StorageConfig) Normalize() {
	switch c.Type {
	case StorageTypeMemory:
		if c.Memory == nil {
			c.Memory = &MemoryStorageConfig{}
		}
	case StorageTypeMongo:
		if c.Mongo == nil {
			c.Mongo = &MongoStorageConfig{}
		}
	case StorageTypeSQL:
		if c.SQL == nil {
			c.SQL = &SQLStorageConfig{}
		}
	}
}

func (c *StorageConfig) storageCases() []validationconfig.StorageCase[StorageType] {
	return []validationconfig.StorageCase[StorageType]{
		{When: StorageTypeMemory, Field: &c.Memory},
		{When: StorageTypeMongo, Field: &c.Mongo},
		{When: StorageTypeSQL, Field: &c.SQL},
	}
}

// Validate ensures the storage type is valid and its configuration is
// provided.
func (c *StorageConfig) Validate() error {
	return validationconfig.ValidateStorage(c, &c.Type, storageAllowedTypes, c.storageCases())
}

// Config configures a retry queue: the backend that persists its items.
//
// Example:
//
//	cfg := &retryqueueconfig.Config{
//		Storage: &retryqueueconfig.StorageConfig{
//			Type: retryqueueconfig.StorageTypeSQL,
//			SQL:  &retryqueueconfig.SQLStorageConfig{Dialect: "postgres", Table: "retry_items"},
//		},
//	}
type Config struct {
	// Storage selects and configures the backend.
	Storage *StorageConfig `yaml:"storage"`
}

// Default returns a retry queue configuration with default values (in-memory
// store).
func Default() Config {
	return Config{Storage: DefaultStorage()}
}

// Normalize prepares nested config for validation by allocating the storage
// sub-config implied by its type. Call before [Config.Validate].
func (c *Config) Normalize() {
	if c.Storage != nil {
		c.Storage.Normalize()
	}
}

// Validate performs validation of the retry queue configuration.
func (c *Config) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Storage, validation.Required),
	)
}

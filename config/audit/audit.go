// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auditconfig

import (
	"slices"
	"time"

	"github.com/altessa-s/go-atlas/config/internal/validators"

	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
	pagingconfig "github.com/altessa-s/go-atlas/config/paging"
	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// StorageType defines the type of storage backend for audit events.
type StorageType string

// Supported audit storage types.
const (
	StorageTypeMemory StorageType = "memory"
	StorageTypeMongo  StorageType = "mongo"
	// StorageTypeClickHouse stores audit events in a ClickHouse table.
	StorageTypeClickHouse StorageType = "clickhouse"
	// StorageTypeSQL stores audit events in a SQL table (PostgreSQL, MySQL or
	// MariaDB) through an injected *sql.DB.
	StorageTypeSQL StorageType = "sqldb"
)

// SQL dialects accepted by [StorageSQL.Dialect].
const (
	// SQLDialectPostgres targets PostgreSQL 12+.
	SQLDialectPostgres = "postgres"
	// SQLDialectMySQL targets MySQL 8.0+ and MariaDB 10.6+.
	SQLDialectMySQL = "mysql"
)

// Config defines the configuration for the audit subsystem.
// Controls how audit events are buffered, dispatched, and persisted.
//
// Dispatch-level tunables (buffer, batch, workers, retries, WAL) are
// delegated to the embedded [Dispatch] under the "dispatch" YAML key.
//
// Example:
//
//	audit := &auditconfig.Config{
//		Enabled: true,
//		Storage: Storage{
//			Type: auditconfig.StorageTypeMongo,
//			Mongo: &StorageMongo{
//				CollectionName: "audit_events",
//			},
//		},
//		Dispatch: Dispatch{
//			BufferSize:    20000,
//			BatchSize:     200,
//			FlushInterval: 500 * time.Millisecond,
//			Workers:       4,
//			WAL: &WAL{Enabled: true, Dir: "./var/audit/wal"},
//		},
//	}
type Config struct {
	// Enabled controls whether the auditor is created.
	// When false, the factory returns nil, nil.
	Enabled bool `yaml:"enabled" default:"true"`

	// Storage defines the storage backend for audit events.
	Storage Storage `yaml:"storage"`

	// ShutdownTimeout is the maximum time to wait during graceful shutdown.
	ShutdownTimeout time.Duration `yaml:"shutdownTimeout" default:"30s"`

	// Dispatch holds the async dispatch engine configuration (buffer,
	// batch, workers, retries, WAL).
	Dispatch dispatchconfig.Config `yaml:"dispatch"`

	// Paging configures signed page tokens for querying events. Nil disables
	// page tokens: queries return a single page.
	Paging *pagingconfig.Config `yaml:"paging" default:"-"`
}

// Storage defines the storage backend configuration for audit events.
type Storage struct {
	validators.BackendTypes[StorageType] `yaml:"-" json:"-"`

	// Type selects the storage backend.
	Type StorageType `yaml:"type" default:"memory"`

	// Mongo holds MongoDB-specific storage configuration.
	// Required when Type is "mongo".
	Mongo *StorageMongo `yaml:"mongo"`

	// ClickHouse holds ClickHouse-specific storage configuration.
	// Required when Type is "clickhouse".
	ClickHouse *StorageClickHouse `yaml:"clickhouse"`

	// SQL holds SQL-specific storage configuration.
	// Required when Type is "sqldb".
	SQL *StorageSQL `yaml:"sqldb"`
}

// StorageSQL holds SQL-specific configuration for audit event storage
// (audit/storages/sqldb). The *sql.DB itself is injected into the
// factory; the caller chooses and registers the driver.
type StorageSQL struct {
	// Dialect selects the SQL flavor: "postgres" or "mysql" (MySQL 8.0+ and
	// MariaDB 10.6+).
	Dialect string `yaml:"dialect" default:"postgres"`

	// TableName is the table holding audit events, optionally
	// schema-qualified.
	TableName string `yaml:"tableName" default:"audit_events"`

	// MaxBatchRows is the largest number of rows sent in a single INSERT; a
	// larger batch is split into INSERTs of one transaction.
	MaxBatchRows int `yaml:"maxBatchRows" default:"500"`

	// TTL is the time-to-live for audit events. Zero means no expiration.
	// Positive values enable the storage's background retention loop.
	TTL time.Duration `yaml:"ttl"`

	// PurgeInterval is the time between background purge passes. Zero disables
	// the loop; explicit PurgeExpired calls remain available.
	PurgeInterval time.Duration `yaml:"purgeInterval" default:"1m"`
	// PurgeBatchSize bounds the rows deleted by one purge statement.
	PurgeBatchSize int `yaml:"purgeBatchSize" default:"1000"`

	// EnsureSchema creates the table and its indexes on startup through the
	// storage's idempotent EnsureSchema. Off by default: schema changes
	// belong in a migration.
	EnsureSchema bool `yaml:"ensureSchema"`
}

// Validate performs validation of the SQL storage configuration.
func (c *StorageSQL) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Dialect, validation.Required, validation.In(SQLDialectPostgres, SQLDialectMySQL)),
		validation.Field(&c.TableName, validation.Required, validators.SQLTableName),
		validation.Field(&c.MaxBatchRows, validation.Min(0)),
		validation.Field(&c.PurgeInterval, validation.Min(time.Duration(0))),
		validation.Field(&c.PurgeBatchSize, validation.Min(1)),
	)
}

// StorageClickHouse holds ClickHouse-specific configuration for audit event
// storage.
type StorageClickHouse struct {
	// TableName is the table holding audit events.
	TableName string `yaml:"tableName" default:"audit_events"`

	// Engine is the table engine used when the table is created.
	// ReplacingMergeTree collapses the duplicate rows that at-least-once
	// delivery produces.
	Engine string `yaml:"engine" default:"ReplacingMergeTree"`

	// Cluster adds an ON CLUSTER clause when the table is created. Empty means
	// a single-node table. ON CLUSTER only distributes the DDL, so pair it
	// with a Replicated* Engine for a replicated deployment.
	Cluster string `yaml:"cluster"`

	// TTL is the retention period for audit events. Zero means no expiration.
	// Applied only when the table is created.
	TTL time.Duration `yaml:"ttl"`

	// MaxBatchSize is the largest number of rows sent in a single INSERT.
	MaxBatchSize int `yaml:"maxBatchSize" default:"10000"`

	// DDLTimeout bounds each startup schema operation: creating the table
	// and the schema check.
	DDLTimeout time.Duration `yaml:"ddlTimeout" default:"30s"`

	// AutoCreateTable creates the table on startup. Off by default: schema
	// changes belong in a migration.
	AutoCreateTable bool `yaml:"autoCreateTable"`

	// Final adds the FINAL modifier to reads so they observe the deduplicated
	// view, at the cost of merging parts at query time.
	Final bool `yaml:"final"`

	// TimeRangeMode is the reaction to a query that bounds no time range:
	// enforce, warn or disabled.
	TimeRangeMode string `yaml:"timeRangeMode" default:"warn"`

	// SchemaCheckMode is the reaction to a table that drifted from the
	// expected schema (missing or differently defined columns and indexes,
	// another sorting or partition key): enforce, warn or disabled.
	SchemaCheckMode string `yaml:"schemaCheckMode" default:"warn"`
}

// Validate performs validation of the ClickHouse storage configuration.
func (c *StorageClickHouse) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.TableName, validation.Required),
		validation.Field(&c.Engine, validation.Required),
		validation.Field(&c.MaxBatchSize, validation.Required, validation.Min(1)),
		validation.Field(&c.DDLTimeout, validation.Min(time.Duration(0))),
		validation.Field(&c.TimeRangeMode, validation.In("enforce", "warn", "disabled")),
		validation.Field(&c.SchemaCheckMode, validation.In("enforce", "warn", "disabled")),
	)
}

// StorageMongo holds MongoDB-specific configuration for audit event storage.
type StorageMongo struct {
	// CollectionName is the MongoDB collection name for audit events.
	CollectionName string `yaml:"collectionName" default:"audit_events"`

	// IndexTimeout is the timeout for index creation operations.
	IndexTimeout time.Duration `yaml:"indexTimeout" default:"30s"`

	// TTL is the time-to-live for audit events. Zero means no expiration.
	TTL time.Duration `yaml:"ttl"`
}

// Validate performs validation of the audit configuration.
func (a *Config) Validate() error {
	return validationconfig.ValidateStruct(a,
		validationconfig.NestedField(&a.Storage),
		validationconfig.NestedField(&a.Dispatch),
		validation.Field(&a.Paging),
	)
}

// Validate performs validation of the audit storage configuration.
func (s *Storage) Validate() error {
	return validationconfig.ValidateStruct(s,
		validation.Field(&s.Type, validation.Required, validation.When(!slices.Contains(s.AllowedTypes(), s.Type),
			validation.In(StorageTypeMemory, StorageTypeMongo, StorageTypeClickHouse, StorageTypeSQL))),
		validation.Field(&s.Mongo, validation.When(s.Type == StorageTypeMongo, validation.Required)),
		validation.Field(&s.ClickHouse, validation.When(s.Type == StorageTypeClickHouse, validation.Required)),
		validation.Field(&s.SQL, validation.When(s.Type == StorageTypeSQL, validation.Required)),
	)
}

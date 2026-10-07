// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auditconfig

import (
	"time"

	"github.com/altessa-s/go-atlas/core/types/redacted"

	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
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
	Paging *Paging `yaml:"paging" default:"-"`
}

// Storage defines the storage backend configuration for audit events.
type Storage struct {
	// Type selects the storage backend.
	Type StorageType `yaml:"type" default:"memory"`

	// Mongo holds MongoDB-specific storage configuration.
	// Required when Type is "mongo".
	Mongo *StorageMongo `yaml:"mongo"`

	// ClickHouse holds ClickHouse-specific storage configuration.
	// Required when Type is "clickhouse".
	ClickHouse *StorageClickHouse `yaml:"clickhouse"`
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

	// DDLTimeout bounds each startup schema operation: creating the table,
	// the additive migration and the schema check.
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

	// SchemaMigrationMode controls whether missing columns and indexes are
	// added at startup: off or additive. Differently defined ones are never
	// changed.
	SchemaMigrationMode string `yaml:"schemaMigrationMode" default:"off"`
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
		validation.Field(&c.SchemaMigrationMode, validation.In("off", "additive")),
	)
}

// Paging configures the signed page tokens of audit queries. Paging through
// results with page tokens requires it; writing events does not.
type Paging struct {
	// SigningKey signs page tokens: at least 32 bytes, the same on every
	// replica.
	SigningKey redacted.RedactedString `yaml:"signingKey"`

	// PreviousKeys still verify tokens signed before a key rotation.
	PreviousKeys []redacted.RedactedString `yaml:"previousKeys"`

	// TokenTTL is how long a page token stays valid.
	TokenTTL time.Duration `yaml:"tokenTTL" default:"24h"`
}

// minSigningKeyLength mirrors keyset.MinKeyLength; schemas do not import
// runtime packages.
const minSigningKeyLength = 32

// Validate performs validation of the paging configuration.
func (p *Paging) Validate() error {
	return validationconfig.ValidateStruct(p,
		validation.Field(&p.SigningKey, validation.Required, validation.Length(minSigningKeyLength, 0)),
		validation.Field(&p.PreviousKeys, validation.Each(validation.Length(minSigningKeyLength, 0))),
		validation.Field(&p.TokenTTL, validation.Min(time.Duration(0))),
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
		validation.Field(&a.Paging),
	)
}

// Validate performs validation of the audit storage configuration.
func (s *Storage) Validate() error {
	return validationconfig.ValidateStruct(s,
		validation.Field(&s.Type, validation.Required, validation.In(StorageTypeMemory, StorageTypeMongo, StorageTypeClickHouse)),
		validation.Field(&s.Mongo, validation.When(s.Type == StorageTypeMongo, validation.Required)),
		validation.Field(&s.ClickHouse, validation.When(s.Type == StorageTypeClickHouse, validation.Required)),
	)
}

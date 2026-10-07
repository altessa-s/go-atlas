// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sagaconfig

import (
	"time"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Saga configuration. They mirror the orchestrator option
// defaults in data/saga so YAML and code stay in sync.
const (
	defaultSagaExecutionTimeout        = 5 * time.Minute
	defaultSagaLeaseGrace              = 30 * time.Second
	defaultSagaStorageTimeout          = 10 * time.Second
	defaultSagaRecoveryTimeout         = 5 * time.Minute
	defaultSagaStepTimeout             = 30 * time.Second
	defaultSagaSagaTimeout             = 0 // disabled: no per-instance deadline
	defaultSagaMaxStepAttempts         = 3
	defaultSagaStepRetryBaseDelay      = 100 * time.Millisecond
	defaultSagaStepRetryMaxDelay       = 5 * time.Second
	defaultSagaMaxCompensationAttempts = 5
	defaultSagaStepConcurrency         = 0 // use the core IO-bound default
	defaultSagaRecoverySchedule        = "@every 1m"
	defaultSagaRecoveryBatchSize       = 100
	defaultSagaRecoveryTaskID          = "saga-recovery"
)

// StorageType selects the backend that persists saga instances.
type StorageType string

const (
	// StorageTypeMemory keeps instances in process (single-node, tests).
	StorageTypeMemory StorageType = "memory"
	// StorageTypeNATS persists instances in a NATS JetStream KeyValue bucket.
	StorageTypeNATS StorageType = "nats"
	// StorageTypeMongo persists instances in a MongoDB collection.
	StorageTypeMongo StorageType = "mongo"
	// StorageTypeRedis persists instances in Redis (hash + recovery index).
	StorageTypeRedis StorageType = "redis"
	// StorageTypeSQL persists instances in a SQL table (PostgreSQL, MySQL or
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

var sagaStorageAllowedTypes = []StorageType{
	StorageTypeMemory,
	StorageTypeNATS,
	StorageTypeMongo,
	StorageTypeRedis,
	StorageTypeSQL,
}

// MemoryStorageConfig configures the in-process saga store
// (data/saga/storages/memory). The backend has no tunables; the type exists so
// every storage backend has a dedicated, discoverable config section.
type MemoryStorageConfig struct{}

// Validate performs validation of the in-memory saga storage configuration.
func (c *MemoryStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c)
}

// NATSStorageConfig configures the durable saga store backed by a NATS
// JetStream KeyValue bucket (data/saga/storages/nats).
type NATSStorageConfig struct {
	// Bucket is the NATS KeyValue bucket name.
	Bucket string `yaml:"bucket" default:"saga"`
	// MaxAge is the per-key expiry applied to the bucket. It must outlive any
	// saga's running time; zero is widened to a 30-day backstop by the store.
	MaxAge time.Duration `yaml:"maxAge" default:"720h"`
	// MigrateBucketTTL updates a pre-existing bucket whose key TTL differs
	// from MaxAge instead of failing with ErrBucketTTLMismatch. Off by
	// default: the bucket's key TTL expires every instance in it, including
	// those of other processes sharing the bucket.
	MigrateBucketTTL bool `yaml:"migrateBucketTTL"`
	// StrictBucketStorage fails with ErrBucketStorageMismatch when the bucket
	// already exists with another storage type, instead of using it as is
	// with a warning.
	StrictBucketStorage bool `yaml:"strictBucketStorage"`
}

// Validate performs validation of the NATS saga storage configuration.
func (c *NATSStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Bucket, validation.Required),
		validation.Field(&c.MaxAge, validation.When(c.MaxAge != 0, ozzo_rules.Duration())),
	)
}

// MongoStorageConfig configures the durable saga store backed by a MongoDB
// collection (data/saga/storages/mongo).
type MongoStorageConfig struct {
	// Collection is the MongoDB collection that stores saga instances.
	Collection string `yaml:"collection" default:"saga_instances"`
}

// Validate performs validation of the MongoDB saga storage configuration.
func (c *MongoStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Collection, validation.Required),
	)
}

// RedisStorageConfig configures the durable saga store backed by Redis
// (data/saga/storages/redis).
type RedisStorageConfig struct {
	// KeysPrefix is prepended to every Redis key the store writes.
	KeysPrefix string `yaml:"keysPrefix" default:"saga:"`
	// TTL is the per-key expiry applied on every write; zero persists forever.
	TTL time.Duration `yaml:"ttl"`
}

// Validate performs validation of the Redis saga storage configuration.
func (c *RedisStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.KeysPrefix, validation.Required),
		validation.Field(&c.TTL, validation.When(c.TTL != 0, ozzo_rules.Duration())),
	)
}

// SQLStorageConfig configures the durable saga store backed by a SQL table
// (data/saga/storages/sqldb). The *sql.DB itself is injected into the factory;
// the caller chooses and registers the driver.
type SQLStorageConfig struct {
	// Dialect selects the SQL flavor: "postgres" or "mysql" (MySQL 8.0+ and
	// MariaDB 10.6+).
	Dialect string `yaml:"dialect" default:"postgres"`
	// Table is the table that stores saga instances, optionally
	// schema-qualified.
	Table string `yaml:"table" default:"saga_instances"`
	// EnsureSchema makes the factory create the table and its indexes while
	// building the orchestrator, through the store's idempotent EnsureSchema.
	// Leave it false when the schema is applied through migrations.
	EnsureSchema bool `yaml:"ensureSchema" default:"false"`
}

// Validate performs validation of the SQL saga storage configuration.
func (c *SQLStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Dialect, validation.Required, ozzo_rules.OneOf(SQLDialectPostgres, SQLDialectMySQL)),
		validation.Field(&c.Table, validation.Required),
	)
}

// StorageConfig selects and configures the saga state-store backend. Each
// backend has its own nested section; only the one named by Type is used.
type StorageConfig struct {
	// Type defines the storage backend type (memory, nats, mongo, redis, or
	// sqldb).
	Type StorageType `yaml:"type" default:"memory"`

	// Memory defines the in-memory configuration.
	// Required when Type is StorageTypeMemory, ignored otherwise.
	Memory *MemoryStorageConfig `yaml:"memory" default:"-"`

	// Nats defines the NATS KeyValue configuration.
	// Required when Type is StorageTypeNATS, ignored otherwise.
	NATS *NATSStorageConfig `yaml:"nats" default:"-"`

	// Mongo defines the MongoDB configuration.
	// Required when Type is StorageTypeMongo, ignored otherwise.
	Mongo *MongoStorageConfig `yaml:"mongo" default:"-"`

	// Redis defines the Redis configuration.
	// Required when Type is StorageTypeRedis, ignored otherwise.
	Redis *RedisStorageConfig `yaml:"redis" default:"-"`

	// SQL defines the SQL database configuration.
	// Required when Type is StorageTypeSQL, ignored otherwise.
	SQL *SQLStorageConfig `yaml:"sqldb" default:"-"`
}

// DefaultStorage returns the default saga storage configuration (in-memory).
func DefaultStorage() *StorageConfig {
	return &StorageConfig{
		Type:   StorageTypeMemory,
		Memory: &MemoryStorageConfig{},
	}
}

// Normalize allocates the provider-specific sub-config implied by
// [StorageConfig.Type]. Call Normalize before Validate so the correct
// sub-struct is present for validation.
func (c *StorageConfig) Normalize() {
	switch c.Type {
	case StorageTypeMemory:
		if c.Memory == nil {
			c.Memory = &MemoryStorageConfig{}
		}
	case StorageTypeNATS:
		if c.NATS == nil {
			c.NATS = &NATSStorageConfig{}
		}
	case StorageTypeMongo:
		if c.Mongo == nil {
			c.Mongo = &MongoStorageConfig{}
		}
	case StorageTypeRedis:
		if c.Redis == nil {
			c.Redis = &RedisStorageConfig{}
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
		{When: StorageTypeNATS, Field: &c.NATS},
		{When: StorageTypeMongo, Field: &c.Mongo},
		{When: StorageTypeRedis, Field: &c.Redis},
		{When: StorageTypeSQL, Field: &c.SQL},
	}
}

// Validate performs validation of the saga storage configuration. Ensures the
// storage type is valid and its corresponding configuration is provided.
func (c *StorageConfig) Validate() error {
	return validationconfig.ValidateStorage(c, &c.Type, sagaStorageAllowedTypes, c.storageCases())
}

// Config configures the saga orchestrator and its state store. It mirrors the
// data/saga orchestrator options plus the storage-backend selection.
//
// Example:
//
//	cfg := &sagaconfig.Config{
//		Storage: &sagaconfig.StorageConfig{
//			Type: sagaconfig.StorageTypeNATS,
//			NATS: &sagaconfig.NATSStorageConfig{Bucket: "saga"},
//		},
//		StepTimeout: 10 * time.Second,
//		SagaTimeout: 5 * time.Minute,
//	}
type Config struct {
	// Storage selects and configures the state-store backend.
	Storage *StorageConfig `yaml:"storage"`

	// StepTimeout bounds a single step action or compensation invocation.
	StepTimeout time.Duration `yaml:"stepTimeout" default:"30s"`
	// ExecutionTimeout bounds one Start/Resume ownership interval.
	ExecutionTimeout time.Duration `yaml:"executionTimeout" default:"5m"`
	// LeaseGrace covers cancellation and clock skew after execution expiry.
	LeaseGrace time.Duration `yaml:"leaseGrace" default:"30s"`
	// StorageTimeout bounds each persistence or scheduler registration operation.
	StorageTimeout time.Duration `yaml:"storageTimeout" default:"10s"`
	// RecoveryTimeout bounds a complete recovery cycle.
	RecoveryTimeout time.Duration `yaml:"recoveryTimeout" default:"5m"`
	// SagaTimeout is the per-instance deadline enabling auto-rollback by the
	// recovery cycle. Zero disables it.
	SagaTimeout time.Duration `yaml:"sagaTimeout" default:"0s"`
	// MaxStepAttempts is the total number of attempts for a forward step action.
	MaxStepAttempts int `yaml:"maxStepAttempts" default:"3"`
	// StepRetryBaseDelay is the first exponential-backoff delay between retries.
	StepRetryBaseDelay time.Duration `yaml:"stepRetryBaseDelay" default:"100ms"`
	// StepRetryMaxDelay caps the step-retry backoff.
	StepRetryMaxDelay time.Duration `yaml:"stepRetryMaxDelay" default:"5s"`
	// MaxCompensationAttempts is the total number of attempts for a compensation.
	MaxCompensationAttempts int `yaml:"maxCompensationAttempts" default:"5"`
	// StepConcurrency bounds parallel-group fan-out (0 = core default).
	StepConcurrency int `yaml:"stepConcurrency" default:"0"`

	// RecoverySchedule is the cron expression for the background recovery cycle.
	RecoverySchedule string `yaml:"recoverySchedule" default:"@every 1m"`
	// RecoveryBatchSize is the maximum instances processed per recovery cycle.
	RecoveryBatchSize int `yaml:"recoveryBatchSize" default:"100"`
	// RecoveryTaskID is the scheduler task ID for the recovery cycle.
	RecoveryTaskID string `yaml:"recoveryTaskID" default:"saga-recovery"`
}

// Default returns a Saga configuration with default values (in-memory store).
func Default() Config {
	return Config{
		Storage:                 DefaultStorage(),
		ExecutionTimeout:        defaultSagaExecutionTimeout,
		LeaseGrace:              defaultSagaLeaseGrace,
		StorageTimeout:          defaultSagaStorageTimeout,
		RecoveryTimeout:         defaultSagaRecoveryTimeout,
		StepTimeout:             defaultSagaStepTimeout,
		SagaTimeout:             defaultSagaSagaTimeout,
		MaxStepAttempts:         defaultSagaMaxStepAttempts,
		StepRetryBaseDelay:      defaultSagaStepRetryBaseDelay,
		StepRetryMaxDelay:       defaultSagaStepRetryMaxDelay,
		MaxCompensationAttempts: defaultSagaMaxCompensationAttempts,
		StepConcurrency:         defaultSagaStepConcurrency,
		RecoverySchedule:        defaultSagaRecoverySchedule,
		RecoveryBatchSize:       defaultSagaRecoveryBatchSize,
		RecoveryTaskID:          defaultSagaRecoveryTaskID,
	}
}

// Normalize prepares nested config for validation by allocating the
// storage sub-config implied by its type. Call before [Config.Validate].
func (s *Config) Normalize() {
	if s.Storage != nil {
		s.Storage.Normalize()
	}
}

// Validate performs validation of the Saga configuration. Returns an error if
// validation fails, nil otherwise.
func (s *Config) Validate() error {
	return validationconfig.ValidateStruct(s,
		validation.Field(&s.Storage, validation.Required),
		validation.Field(&s.ExecutionTimeout, validation.When(s.ExecutionTimeout != 0, ozzo_rules.Duration())),
		validation.Field(&s.LeaseGrace, validation.When(s.LeaseGrace != 0, ozzo_rules.Duration())),
		validation.Field(&s.StorageTimeout, validation.When(s.StorageTimeout != 0, ozzo_rules.Duration())),
		validation.Field(&s.RecoveryTimeout, validation.When(s.RecoveryTimeout != 0, ozzo_rules.Duration())),
		validation.Field(&s.StepTimeout, validation.When(s.StepTimeout != 0, ozzo_rules.Duration())),
		validation.Field(&s.SagaTimeout, validation.When(s.SagaTimeout != 0, ozzo_rules.Duration())),
		// Required is paired with Min because ozzo-validation skips every
		// rule but Required for a zero value — Min(1) alone accepts 0.
		validation.Field(&s.MaxStepAttempts, validation.Required, validation.Min(1)),
		validation.Field(&s.StepRetryBaseDelay, validation.When(s.StepRetryBaseDelay != 0, ozzo_rules.Duration())),
		validation.Field(&s.StepRetryMaxDelay, validation.When(s.StepRetryMaxDelay != 0, ozzo_rules.Duration())),
		validation.Field(&s.MaxCompensationAttempts, validation.Required, validation.Min(1)),
		validation.Field(&s.StepConcurrency, validation.Min(0)),
		validation.Field(&s.RecoverySchedule, validation.Required),
		validation.Field(&s.RecoveryBatchSize, validation.Required, validation.Min(1)),
		validation.Field(&s.RecoveryTaskID, validation.Required),
	)
}

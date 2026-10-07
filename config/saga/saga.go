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
	defaultSagaStoreTimeout            = 10 * time.Second
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
)

var sagaStorageAllowedTypes = []StorageType{
	StorageTypeMemory,
	StorageTypeNATS,
	StorageTypeMongo,
	StorageTypeRedis,
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
	MaxAge time.Duration `yaml:"max_age" default:"720h"`
	// MigrateBucketTTL updates a pre-existing bucket whose key TTL differs
	// from MaxAge instead of failing with ErrBucketTTLMismatch. Off by
	// default: the bucket's key TTL expires every instance in it, including
	// those of other processes sharing the bucket.
	MigrateBucketTTL bool `yaml:"migrate_bucket_ttl"`
	// StrictBucketStorage fails with ErrBucketStorageMismatch when the bucket
	// already exists with another storage type, instead of using it as is
	// with a warning.
	StrictBucketStorage bool `yaml:"strict_bucket_storage"`
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
	KeysPrefix string `yaml:"keys_prefix" default:"saga:"`
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

// StorageConfig selects and configures the saga state-store backend. Each
// backend has its own nested section; only the one named by Type is used.
type StorageConfig struct {
	// Type defines the storage backend type (memory, nats, mongo, or redis).
	Type StorageType `yaml:"type" default:"memory"`

	// Memory defines the in-memory configuration.
	// Required when Type is StorageTypeMemory, ignored otherwise.
	Memory *MemoryStorageConfig `yaml:"memory" default:"-"`

	// Nats defines the NATS KeyValue configuration.
	// Required when Type is StorageTypeNATS, ignored otherwise.
	Nats *NATSStorageConfig `yaml:"nats" default:"-"`

	// Mongo defines the MongoDB configuration.
	// Required when Type is StorageTypeMongo, ignored otherwise.
	Mongo *MongoStorageConfig `yaml:"mongo" default:"-"`

	// Redis defines the Redis configuration.
	// Required when Type is StorageTypeRedis, ignored otherwise.
	Redis *RedisStorageConfig `yaml:"redis" default:"-"`
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
		if c.Nats == nil {
			c.Nats = &NATSStorageConfig{}
		}
	case StorageTypeMongo:
		if c.Mongo == nil {
			c.Mongo = &MongoStorageConfig{}
		}
	case StorageTypeRedis:
		if c.Redis == nil {
			c.Redis = &RedisStorageConfig{}
		}
	}
}

func (c *StorageConfig) storageCases() []validationconfig.StorageCase[StorageType] {
	return []validationconfig.StorageCase[StorageType]{
		{When: StorageTypeMemory, Field: &c.Memory},
		{When: StorageTypeNATS, Field: &c.Nats},
		{When: StorageTypeMongo, Field: &c.Mongo},
		{When: StorageTypeRedis, Field: &c.Redis},
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
//			Nats: &sagaconfig.NATSStorageConfig{Bucket: "saga"},
//		},
//		StepTimeout: 10 * time.Second,
//		SagaTimeout: 5 * time.Minute,
//	}
type Config struct {
	// Storage selects and configures the state-store backend.
	Storage *StorageConfig `yaml:"storage"`

	// StepTimeout bounds a single step action or compensation invocation.
	StepTimeout time.Duration `yaml:"step_timeout" default:"30s"`
	// ExecutionTimeout bounds one Start/Resume ownership interval.
	ExecutionTimeout time.Duration `yaml:"execution_timeout" default:"5m"`
	// LeaseGrace covers cancellation and clock skew after execution expiry.
	LeaseGrace time.Duration `yaml:"lease_grace" default:"30s"`
	// StoreTimeout bounds each persistence or scheduler registration operation.
	StoreTimeout time.Duration `yaml:"store_timeout" default:"10s"`
	// RecoveryTimeout bounds a complete recovery cycle.
	RecoveryTimeout time.Duration `yaml:"recovery_timeout" default:"5m"`
	// SagaTimeout is the per-instance deadline enabling auto-rollback by the
	// recovery cycle. Zero disables it.
	SagaTimeout time.Duration `yaml:"saga_timeout" default:"0s"`
	// MaxStepAttempts is the total number of attempts for a forward step action.
	MaxStepAttempts int `yaml:"max_step_attempts" default:"3"`
	// StepRetryBaseDelay is the first exponential-backoff delay between retries.
	StepRetryBaseDelay time.Duration `yaml:"step_retry_base_delay" default:"100ms"`
	// StepRetryMaxDelay caps the step-retry backoff.
	StepRetryMaxDelay time.Duration `yaml:"step_retry_max_delay" default:"5s"`
	// MaxCompensationAttempts is the total number of attempts for a compensation.
	MaxCompensationAttempts int `yaml:"max_compensation_attempts" default:"5"`
	// StepConcurrency bounds parallel-group fan-out (0 = core default).
	StepConcurrency int `yaml:"step_concurrency" default:"0"`

	// RecoverySchedule is the cron expression for the background recovery cycle.
	RecoverySchedule string `yaml:"recovery_schedule" default:"@every 1m"`
	// RecoveryBatchSize is the maximum instances processed per recovery cycle.
	RecoveryBatchSize int `yaml:"recovery_batch_size" default:"100"`
	// RecoveryTaskID is the scheduler task ID for the recovery cycle.
	RecoveryTaskID string `yaml:"recovery_task_id" default:"saga-recovery"`
}

// Default returns a Saga configuration with default values (in-memory store).
func Default() Config {
	return Config{
		Storage:                 DefaultStorage(),
		ExecutionTimeout:        defaultSagaExecutionTimeout,
		LeaseGrace:              defaultSagaLeaseGrace,
		StoreTimeout:            defaultSagaStoreTimeout,
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
		validation.Field(&s.StoreTimeout, validation.When(s.StoreTimeout != 0, ozzo_rules.Duration())),
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

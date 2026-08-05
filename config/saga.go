// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Saga configuration. They mirror the orchestrator option
// defaults in data/saga so YAML and code stay in sync.
const (
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

// SagaStorageType selects the backend that persists saga instances.
type SagaStorageType string

const (
	// SagaStorageTypeMemory keeps instances in process (single-node, tests).
	SagaStorageTypeMemory SagaStorageType = "memory"
	// SagaStorageTypeNats persists instances in a NATS JetStream KeyValue bucket.
	SagaStorageTypeNats SagaStorageType = "nats"
	// SagaStorageTypeMongo persists instances in a MongoDB collection.
	SagaStorageTypeMongo SagaStorageType = "mongo"
	// SagaStorageTypeRedis persists instances in Redis (hash + recovery index).
	SagaStorageTypeRedis SagaStorageType = "redis"
)

var sagaStorageAllowedTypes = []SagaStorageType{
	SagaStorageTypeMemory,
	SagaStorageTypeNats,
	SagaStorageTypeMongo,
	SagaStorageTypeRedis,
}

// SagaMemoryStorageConfig configures the in-process saga store
// (data/saga/storages/memory). The backend has no tunables; the type exists so
// every storage backend has a dedicated, discoverable config section.
type SagaMemoryStorageConfig struct{}

// Validate performs validation of the in-memory saga storage configuration.
func (c *SagaMemoryStorageConfig) Validate() error {
	return ValidateStruct(c)
}

// SagaNatsStorageConfig configures the durable saga store backed by a NATS
// JetStream KeyValue bucket (data/saga/storages/nats).
type SagaNatsStorageConfig struct {
	// Bucket is the NATS KeyValue bucket name.
	Bucket string `yaml:"bucket" default:"saga"`
	// MaxAge is the per-key expiry applied to the bucket. It must outlive any
	// saga's running time; zero is widened to a 30-day backstop by the store.
	MaxAge time.Duration `yaml:"max_age" default:"720h"`
}

// Validate performs validation of the NATS saga storage configuration.
func (c *SagaNatsStorageConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Bucket, validation.Required),
		validation.Field(&c.MaxAge, validation.When(c.MaxAge != 0, ozzo_rules.Duration())),
	)
}

// SagaMongoStorageConfig configures the durable saga store backed by a MongoDB
// collection (data/saga/storages/mongo).
type SagaMongoStorageConfig struct {
	// Collection is the MongoDB collection that stores saga instances.
	Collection string `yaml:"collection" default:"saga_instances"`
}

// Validate performs validation of the MongoDB saga storage configuration.
func (c *SagaMongoStorageConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Collection, validation.Required),
	)
}

// SagaRedisStorageConfig configures the durable saga store backed by Redis
// (data/saga/storages/redis).
type SagaRedisStorageConfig struct {
	// KeysPrefix is prepended to every Redis key the store writes.
	KeysPrefix string `yaml:"keys_prefix" default:"saga:"`
	// TTL is the per-key expiry applied on every write; zero persists forever.
	TTL time.Duration `yaml:"ttl"`
}

// Validate performs validation of the Redis saga storage configuration.
func (c *SagaRedisStorageConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.KeysPrefix, validation.Required),
		validation.Field(&c.TTL, validation.When(c.TTL != 0, ozzo_rules.Duration())),
	)
}

// SagaStorageConfig selects and configures the saga state-store backend. Each
// backend has its own nested section; only the one named by Type is used.
type SagaStorageConfig struct {
	// Type defines the storage backend type (memory, nats, mongo, or redis).
	Type SagaStorageType `yaml:"type" default:"memory"`

	// Memory defines the in-memory configuration.
	// Required when Type is SagaStorageTypeMemory, ignored otherwise.
	Memory *SagaMemoryStorageConfig `yaml:"memory" default:"-"`

	// Nats defines the NATS KeyValue configuration.
	// Required when Type is SagaStorageTypeNats, ignored otherwise.
	Nats *SagaNatsStorageConfig `yaml:"nats" default:"-"`

	// Mongo defines the MongoDB configuration.
	// Required when Type is SagaStorageTypeMongo, ignored otherwise.
	Mongo *SagaMongoStorageConfig `yaml:"mongo" default:"-"`

	// Redis defines the Redis configuration.
	// Required when Type is SagaStorageTypeRedis, ignored otherwise.
	Redis *SagaRedisStorageConfig `yaml:"redis" default:"-"`
}

// DefaultSagaStorage returns the default saga storage configuration (in-memory).
func DefaultSagaStorage() *SagaStorageConfig {
	return &SagaStorageConfig{
		Type:   SagaStorageTypeMemory,
		Memory: &SagaMemoryStorageConfig{},
	}
}

// Normalize allocates the provider-specific sub-config implied by
// [SagaStorageConfig.Type]. Call Normalize before Validate so the correct
// sub-struct is present for validation.
func (c *SagaStorageConfig) Normalize() {
	switch c.Type {
	case SagaStorageTypeMemory:
		if c.Memory == nil {
			c.Memory = &SagaMemoryStorageConfig{}
		}
	case SagaStorageTypeNats:
		if c.Nats == nil {
			c.Nats = &SagaNatsStorageConfig{}
		}
	case SagaStorageTypeMongo:
		if c.Mongo == nil {
			c.Mongo = &SagaMongoStorageConfig{}
		}
	case SagaStorageTypeRedis:
		if c.Redis == nil {
			c.Redis = &SagaRedisStorageConfig{}
		}
	}
}

func (c *SagaStorageConfig) storageCases() []storageCase[SagaStorageType] {
	return []storageCase[SagaStorageType]{
		{when: SagaStorageTypeMemory, field: &c.Memory},
		{when: SagaStorageTypeNats, field: &c.Nats},
		{when: SagaStorageTypeMongo, field: &c.Mongo},
		{when: SagaStorageTypeRedis, field: &c.Redis},
	}
}

// Validate performs validation of the saga storage configuration. Ensures the
// storage type is valid and its corresponding configuration is provided.
func (c *SagaStorageConfig) Validate() error {
	return validateStorageConfig(c, &c.Type, sagaStorageAllowedTypes, c.storageCases())
}

// Saga configures the saga orchestrator and its state store. It mirrors the
// data/saga orchestrator options plus the storage-backend selection.
//
// Example:
//
//	cfg := &config.Saga{
//		Storage: &config.SagaStorageConfig{
//			Type: config.SagaStorageTypeNats,
//			Nats: &config.SagaNatsStorageConfig{Bucket: "saga"},
//		},
//		StepTimeout: 10 * time.Second,
//		SagaTimeout: 5 * time.Minute,
//	}
type Saga struct {
	// Storage selects and configures the state-store backend.
	Storage *SagaStorageConfig `yaml:"storage"`

	// StepTimeout bounds a single step action or compensation invocation.
	StepTimeout time.Duration `yaml:"step_timeout" default:"30s"`
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

// DefaultSaga returns a Saga configuration with default values (in-memory store).
func DefaultSaga() Saga {
	return Saga{
		Storage:                 DefaultSagaStorage(),
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
// storage sub-config implied by its type. Call before [Saga.Validate].
func (s *Saga) Normalize() {
	if s.Storage != nil {
		s.Storage.Normalize()
	}
}

// Validate performs validation of the Saga configuration. Returns an error if
// validation fails, nil otherwise.
func (s *Saga) Validate() error {
	return ValidateStruct(s,
		validation.Field(&s.Storage, validation.Required),
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

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyconfig

import (
	"time"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Idempotency configuration.
const (
	defaultIdempotencyTTL = 24 * time.Hour
)

// Config defines the configuration for idempotency key management.
// Controls how idempotency keys are stored and managed for duplicate request detection.
//
// Example:
//
//	idem := &idempotencyconfig.Config{
//		TTL: 24 * time.Hour,
//		Storage: &idempotencyconfig.StorageConfig{
//			Type: idempotencyconfig.StorageTypeRedis,
//		},
//	}
type Config struct {
	// TTL defines the time-to-live for idempotency keys.
	// Keys are automatically expired after this duration to prevent storage bloat.
	TTL time.Duration `yaml:"ttl" default:"24h"`

	// MaxLockDuration is the threshold for treating an in-progress
	// lock as orphaned. AttemptLock callers that observe a stale
	// InProgress entry older than this attempt a CAS-steal. When unset
	// (zero), the Keeper logs a one-time warning at construction and
	// falls back to the package-level default
	// (idempotency.DefaultMaxLockDuration, 5m). Set explicitly to
	// silence the warning and document an evaluated value for this
	// service.
	MaxLockDuration time.Duration `yaml:"maxLockDuration"`

	// Storage defines the storage configuration for idempotency keys.
	// Required, specifies backend and settings.
	Storage *StorageConfig `yaml:"storage"`
}

// StorageType selects the backend that stores idempotency keys.
type StorageType string

const (
	// StorageTypeMemory keeps keys in process memory (single instance).
	StorageTypeMemory StorageType = "memory"
	// StorageTypeRedis stores keys in Redis.
	StorageTypeRedis StorageType = "redis"
	// StorageTypeNATS stores keys in a NATS KeyValue bucket.
	StorageTypeNATS StorageType = "nats"
	// StorageTypeMongo stores keys in a MongoDB collection; operations join
	// the caller's transaction, so a key commits with the business change.
	StorageTypeMongo StorageType = "mongo"
)

var storageAllowedTypes = []StorageType{StorageTypeMemory, StorageTypeRedis, StorageTypeNATS, StorageTypeMongo}

// MongoStorageConfig configures the MongoDB key store
// (data/idempotency/storages/mongo).
type MongoStorageConfig struct {
	// Collection is the MongoDB collection that stores the keys.
	Collection string `yaml:"collection" default:"idempotency_keys"`
}

// Validate performs validation of the MongoDB storage configuration.
func (c *MongoStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Collection, validation.Required),
	)
}

// StorageConfig selects and configures the idempotency key store. Each
// backend has its own nested section; only the one named by Type is used.
type StorageConfig struct {
	// Type defines the storage backend type (memory, redis, nats or mongo).
	Type StorageType `yaml:"type"`

	// Memory defines the in-memory configuration.
	// Required when Type is StorageTypeMemory, ignored otherwise.
	Memory *storageconfig.MemoryConfig `yaml:"memory" default:"-"`

	// NATS defines the NATS KeyValue configuration.
	// Required when Type is StorageTypeNATS, ignored otherwise.
	NATS *storageconfig.NATSConfig `yaml:"nats" default:"-"`

	// Redis defines the Redis configuration.
	// Required when Type is StorageTypeRedis, ignored otherwise.
	Redis *storageconfig.RedisConfig `yaml:"redis" default:"-"`

	// Mongo defines the MongoDB configuration.
	// Required when Type is StorageTypeMongo, ignored otherwise.
	Mongo *MongoStorageConfig `yaml:"mongo" default:"-"`
}

// Normalize allocates the provider-specific sub-config implied by
// [StorageConfig.Type]. Call Normalize before Validate so the correct
// sub-struct is present for validation.
func (c *StorageConfig) Normalize() {
	switch c.Type {
	case StorageTypeMemory:
		if c.Memory == nil {
			c.Memory = &storageconfig.MemoryConfig{}
		}
	case StorageTypeNATS:
		if c.NATS == nil {
			c.NATS = &storageconfig.NATSConfig{}
		}
	case StorageTypeRedis:
		if c.Redis == nil {
			c.Redis = &storageconfig.RedisConfig{}
		}
	case StorageTypeMongo:
		if c.Mongo == nil {
			c.Mongo = &MongoStorageConfig{}
		}
	}
}

func (c *StorageConfig) storageCases() []validationconfig.StorageCase[StorageType] {
	return []validationconfig.StorageCase[StorageType]{
		{When: StorageTypeMemory, Field: &c.Memory},
		{When: StorageTypeNATS, Field: &c.NATS},
		{When: StorageTypeRedis, Field: &c.Redis},
		{When: StorageTypeMongo, Field: &c.Mongo},
	}
}

// Validate ensures the storage type is valid and its configuration is
// provided.
func (c *StorageConfig) Validate() error {
	return validationconfig.ValidateStorage(c, &c.Type, storageAllowedTypes, c.storageCases())
}

// Default returns an Idempotency configuration with default values.
// Note: Storage is left as nil since it is a required field.
func Default() Config {
	return Config{
		TTL: defaultIdempotencyTTL,
	}
}

// Validate performs validation of the idempotency configuration.
// Ensures that TTL and storage are correctly configured.
// Returns an error if validation fails, nil otherwise.
func (i *Config) Validate() error {
	return validationconfig.ValidateStruct(i,
		validation.Field(&i.TTL, validation.Required, ozzo_rules.Duration(), validation.Min(time.Minute)),
		validation.Field(&i.MaxLockDuration, validation.When(i.MaxLockDuration != 0,
			ozzo_rules.Duration(), validation.Min(time.Second))),
		validation.Field(&i.Storage, validation.Required),
	)
}

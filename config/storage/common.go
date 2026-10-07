// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storageconfig

import (
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Storage configuration constants used across different storage backends.
const (
	// MaxNATSReplicas defines the maximum number of NATS KeyValue replicas.
	// This limit prevents excessive replication that could impact performance.
	MaxNATSReplicas = 3

	// MaxKeyPrefixLength defines the maximum length for storage key prefixes.
	// Reasonable limit to prevent storage key bloat and ensure compatibility.
	MaxKeyPrefixLength = 64
)

// KVStorageType selects where a NATS JetStream KeyValue bucket keeps its data.
type KVStorageType string

const (
	// KVStorageMemory keeps the bucket in memory: fast, lost when the
	// JetStream servers holding it stop.
	KVStorageMemory KVStorageType = "memory"
	// KVStorageFile keeps the bucket on disk, surviving server restarts.
	KVStorageFile KVStorageType = "file"
)

// NATSConfig defines common NATS-specific configuration for storage backends.
// Contains settings for NATS KeyValue bucket creation and replication strategy.
//
// TTL is intentionally NOT exposed here — TTL belongs at the feature
// level (e.g. [Idempotency.TTL]) so the same storage struct can be shared
// across features with different lifetime requirements.
type NATSConfig struct {
	// Bucket is the name of the NATS KeyValue bucket.
	// If empty, a default bucket name will be generated based on service name.
	Bucket string `yaml:"bucket"`

	// Replicas defines the number of replicas for NATS KeyValue storage.
	// Higher values provide better availability but increase storage overhead.
	Replicas int `yaml:"replicas" default:"3"`

	// MigrateBucketTTL updates a pre-existing bucket whose key TTL differs
	// from the feature's TTL instead of failing with ErrBucketTTLMismatch.
	// Off by default: the bucket's key TTL expires every key in it, including
	// keys of other processes sharing the bucket.
	MigrateBucketTTL bool `yaml:"migrateBucketTTL"`

	// StrictBucketStorage fails with ErrBucketStorageMismatch when the bucket
	// already exists with another storage type, instead of using it as is
	// with a warning.
	StrictBucketStorage bool `yaml:"strictBucketStorage"`
}

// Validate performs validation of the NATS storage configuration.
// Ensures bucket names and replica counts are within acceptable limits.
func (c *NATSConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Bucket),
		// Required is paired with Min because ozzo-validation skips every
		// rule but Required for a zero value — Min(1) alone accepts 0.
		validation.Field(&c.Replicas, validation.Required, validation.Min(1), validation.Max(MaxNATSReplicas)),
	)
}

// RedisConfig defines common Redis-specific configuration for storage backends.
// Contains Redis-specific settings for key prefixes and storage behavior.
//
// TTL is intentionally NOT exposed here — TTL belongs at the feature
// level (e.g. [Idempotency.TTL]) so the same storage struct can be shared
// across features with different lifetime requirements.
type RedisConfig struct {
	// KeysPrefix is a prefix for all keys in storage.
	// Useful for namespacing when sharing storage between multiple services.
	KeysPrefix string `yaml:"keysPrefix"`
}

// Validate performs validation of the Redis storage configuration.
func (c *RedisConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.KeysPrefix,
			validation.When(c.KeysPrefix != "", validation.Length(0, MaxKeyPrefixLength))),
	)
}

// MemoryConfig defines common in-memory storage configuration.
// Contains settings for memory-based storage with cleanup schedule.
type MemoryConfig struct {
	// CleanupSchedule defines the cron schedule for the cleanup task.
	// Supports standard cron format with seconds: "sec min hour day month weekday"
	// (e.g., "0 */5 * * * *") or descriptor format (e.g., "@every 5m").
	CleanupSchedule string `yaml:"cleanupSchedule" default:"@every 5m"`
}

// Validate performs validation of the memory storage configuration.
func (c *MemoryConfig) Validate() error {
	return validationconfig.ValidateStruct(c)
}

// CacheStorageType defines storage backend types for caching.
// Used by idempotency, rate limiting, and other features that need distributed storage.
type CacheStorageType string

const (
	// CacheStorageTypeMemory represents in-memory storage.
	// Provides fast access but is not shared across instances.
	// Useful for testing or single-instance deployments.
	CacheStorageTypeMemory CacheStorageType = "memory"

	// CacheStorageTypeRedis represents Redis storage.
	// Provides shared storage across instances with persistence options.
	CacheStorageTypeRedis CacheStorageType = "redis"

	// CacheStorageTypeNats represents NATS KeyValue storage.
	// Offers distributed storage with built-in replication and stream features.
	CacheStorageTypeNats CacheStorageType = "nats"
)

var cacheStorageAllowedTypes = []CacheStorageType{
	CacheStorageTypeMemory,
	CacheStorageTypeRedis,
	CacheStorageTypeNats,
}

// CacheStorageConfig defines storage backend configuration with type selector.
// Provides a unified storage configuration for idempotency, rate limiting,
// and other features that require distributed storage.
type CacheStorageConfig struct {
	// Type defines the storage backend type.
	// Must be one of the supported storage types (memory, redis, nats).
	Type CacheStorageType `yaml:"type"`

	// Memory defines the in-memory configuration.
	// Required when Type is CacheStorageTypeMemory, ignored otherwise.
	Memory *MemoryConfig `yaml:"memory" default:"-"`

	// Nats defines the NATS configuration.
	// Required when Type is CacheStorageTypeNats, ignored otherwise.
	Nats *NATSConfig `yaml:"nats" default:"-"`

	// Redis defines the Redis configuration.
	// Required when Type is CacheStorageTypeRedis, ignored otherwise.
	Redis *RedisConfig `yaml:"redis" default:"-"`
}

// Normalize allocates the provider-specific sub-config implied by [CacheStorageConfig.Type].
// Call Normalize before Validate so the correct sub-struct is present for validation.
func (c *CacheStorageConfig) Normalize() {
	switch c.Type {
	case CacheStorageTypeMemory:
		if c.Memory == nil {
			c.Memory = &MemoryConfig{}
		}
	case CacheStorageTypeNats:
		if c.Nats == nil {
			c.Nats = &NATSConfig{}
		}
	case CacheStorageTypeRedis:
		if c.Redis == nil {
			c.Redis = &RedisConfig{}
		}
	}
}

func (c *CacheStorageConfig) storageCases() []validationconfig.StorageCase[CacheStorageType] {
	return []validationconfig.StorageCase[CacheStorageType]{
		{When: CacheStorageTypeMemory, Field: &c.Memory},
		{When: CacheStorageTypeNats, Field: &c.Nats},
		{When: CacheStorageTypeRedis, Field: &c.Redis},
	}
}

// Validate performs validation of the cache storage configuration.
// Ensures storage type is valid and corresponding configuration is provided.
func (c *CacheStorageConfig) Validate() error {
	return validationconfig.ValidateStorage(c, &c.Type, cacheStorageAllowedTypes, c.storageCases())
}

// Enableable is an interface for configurations that can be enabled or disabled.
// Implement this interface to provide consistent enable/disable behavior across configs.
type Enableable interface {
	IsEnabled() bool
}

// IgnoreConfig defines common configuration for method/pattern-based exclusions.
// Used by interceptors to skip processing for certain methods or patterns.
type IgnoreConfig struct {
	// IgnoreMethods is a list of gRPC methods to skip processing for.
	// Methods should be specified in the format "/service.Service/Method".
	IgnoreMethods []string `yaml:"ignoreMethods"`

	// IgnorePatterns is a list of regex patterns for methods to skip processing.
	// Pattern-based rules for bypass.
	IgnorePatterns []string `yaml:"ignorePatterns"`
}

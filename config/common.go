// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
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

// FallbackBehavior defines how interceptors behave when encountering errors
// or when storage backends are unavailable. Used by rate limiting, idempotency,
// and other interceptors that require graceful degradation.
type FallbackBehavior string

const (
	// FallbackBehaviorAllow allows all requests when the underlying service fails.
	// This provides maximum availability but no protection.
	// Use for non-critical services where availability is more important than protection.
	FallbackBehaviorAllow FallbackBehavior = "allow"

	// FallbackBehaviorDeny rejects all requests when the underlying service fails.
	// This provides maximum protection but may impact availability.
	// Use for critical services where protection is essential for stability.
	FallbackBehaviorDeny FallbackBehavior = "deny"

	// FallbackBehaviorError returns an error when the underlying service fails.
	// This is useful for debugging and testing, but not recommended for production.
	FallbackBehaviorError FallbackBehavior = "error"
)

// AllFallbackBehaviors returns all valid FallbackBehavior values.
func AllFallbackBehaviors() []FallbackBehavior {
	return []FallbackBehavior{
		FallbackBehaviorAllow,
		FallbackBehaviorDeny,
		FallbackBehaviorError,
	}
}

// StorageNATSConfig defines common NATS-specific configuration for storage backends.
// Contains settings for NATS KeyValue bucket creation and replication strategy.
type StorageNATSConfig struct {
	// Bucket is the name of the NATS KeyValue bucket.
	// If empty, a default bucket name will be generated based on service name.
	Bucket string `yaml:"bucket,omitempty"`

	// Replicas defines the number of replicas for NATS KeyValue storage.
	// Higher values provide better availability but increase storage overhead.
	Replicas int `yaml:"replicas" default:"3"`
}

// Validate performs validation of the NATS storage configuration.
// Ensures bucket names and replica counts are within acceptable limits.
func (c *StorageNATSConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Bucket),
		validation.Field(&c.Replicas, validation.Min(1), validation.Max(MaxNATSReplicas)),
	)
}

// StorageRedisConfig defines common Redis-specific configuration for storage backends.
// Contains Redis-specific settings for key prefixes and storage behavior.
type StorageRedisConfig struct {
	// KeysPrefix is a prefix for all keys in storage.
	// Useful for namespacing when sharing storage between multiple services.
	KeysPrefix string `yaml:"keysPrefix,omitempty"`
}

// Validate performs validation of the Redis storage configuration.
func (c *StorageRedisConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.KeysPrefix,
			validation.When(c.KeysPrefix != "", validation.Length(0, MaxKeyPrefixLength))),
	)
}

// StorageMemoryConfig defines common in-memory storage configuration.
// Contains settings for memory-based storage with cleanup schedule.
type StorageMemoryConfig struct {
	// CleanupSchedule defines the cron schedule for the cleanup task.
	// Supports standard cron format with seconds: "sec min hour day month weekday"
	// (e.g., "0 */5 * * * *") or descriptor format (e.g., "@every 5m").
	CleanupSchedule string `yaml:"cleanupSchedule" default:"@every 5m"`
}

// Validate performs validation of the memory storage configuration.
func (c *StorageMemoryConfig) Validate() error {
	return ValidateStruct(c)
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
	Memory *StorageMemoryConfig `yaml:"memory,omitempty" default:"-"`

	// Nats defines the NATS configuration.
	// Required when Type is CacheStorageTypeNats, ignored otherwise.
	Nats *StorageNATSConfig `yaml:"nats,omitempty" default:"-"`

	// Redis defines the Redis configuration.
	// Required when Type is CacheStorageTypeRedis, ignored otherwise.
	Redis *StorageRedisConfig `yaml:"redis,omitempty" default:"-"`
}

// Normalize allocates the provider-specific sub-config implied by [CacheStorageConfig.Type].
// Call Normalize before Validate so the correct sub-struct is present for validation.
func (c *CacheStorageConfig) Normalize() {
	switch c.Type {
	case CacheStorageTypeMemory:
		c.Memory = &StorageMemoryConfig{}
	case CacheStorageTypeNats:
		c.Nats = &StorageNATSConfig{}
	case CacheStorageTypeRedis:
		c.Redis = &StorageRedisConfig{}
	}
}

func (c *CacheStorageConfig) storageCases() []storageCase[CacheStorageType] {
	return []storageCase[CacheStorageType]{
		{when: CacheStorageTypeMemory, field: &c.Memory},
		{when: CacheStorageTypeNats, field: &c.Nats},
		{when: CacheStorageTypeRedis, field: &c.Redis},
	}
}

// Validate performs validation of the cache storage configuration.
// Ensures storage type is valid and corresponding configuration is provided.
func (c *CacheStorageConfig) Validate() error {
	return validateStorageConfig(c, &c.Type, cacheStorageAllowedTypes, c.storageCases())
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

// ValidateIgnoreConfig returns validation field rules for IgnoreConfig embedded fields.
// Use this helper to validate IgnoreMethods and IgnorePatterns fields in config structs.
func ValidateIgnoreConfig(ignoreMethods, ignorePatterns *[]string) []*validation.FieldRules {
	return []*validation.FieldRules{
		validation.Field(ignoreMethods, validation.Each(validation.Required)),
		validation.Field(ignorePatterns, validation.Each(validation.Required)),
	}
}

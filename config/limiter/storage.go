// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterconfig

import (
	"github.com/altessa-s/go-atlas/config/internal/validators"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	validationconfig "github.com/altessa-s/go-nomos/validation"
)

// StorageType selects the storage backend of a rate limiter.
type StorageType string

const (
	// StorageTypeMemory represents in-memory storage (single instance).
	StorageTypeMemory StorageType = "memory"

	// StorageTypeRedis represents Redis storage (shared across instances).
	StorageTypeRedis StorageType = "redis"
)

var storageAllowedTypes = []StorageType{
	StorageTypeMemory,
	StorageTypeRedis,
}

// StorageConfig selects and configures a rate limiter's storage backend.
// Built-in backends are memory and redis; custom backends registered on the
// limiter factory can be admitted via SetRegisteredTypes.
type StorageConfig struct {
	validators.BackendTypes[StorageType] `yaml:"-" json:"-"`

	// Type defines the storage backend type.
	// Built-in types are memory and redis.
	Type StorageType `yaml:"type"`

	// Memory defines the in-memory configuration.
	// Required when Type is StorageTypeMemory, ignored otherwise.
	Memory *storageconfig.MemoryConfig `yaml:"memory" default:"-"`

	// Redis defines the Redis configuration.
	// Required when Type is StorageTypeRedis, ignored otherwise.
	Redis *storageconfig.RedisConfig `yaml:"redis" default:"-"`
}

// Normalize allocates the provider-specific sub-config implied by [StorageConfig.Type].
// Call Normalize before Validate so the correct sub-struct is present for validation.
func (c *StorageConfig) Normalize() {
	switch c.Type {
	case StorageTypeMemory:
		if c.Memory == nil {
			c.Memory = &storageconfig.MemoryConfig{}
		}
	case StorageTypeRedis:
		if c.Redis == nil {
			c.Redis = &storageconfig.RedisConfig{}
		}
	}
}

func (c *StorageConfig) storageCases() []validationconfig.StorageCase[StorageType] {
	return []validationconfig.StorageCase[StorageType]{
		{When: StorageTypeMemory, Field: &c.Memory},
		{When: StorageTypeRedis, Field: &c.Redis},
	}
}

// Validate performs validation of the limiter storage configuration.
// Ensures storage type is valid and corresponding configuration is provided.
func (c *StorageConfig) Validate() error {
	return validationconfig.ValidateStorage(c, &c.Type, c.AllowedTypes(storageAllowedTypes...), c.storageCases())
}

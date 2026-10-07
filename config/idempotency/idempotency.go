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
//		Storage: &storageconfig.CacheStorageConfig{
//			Type: storageconfig.CacheStorageTypeRedis,
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
	Storage *storageconfig.CacheStorageConfig `yaml:"storage"`
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

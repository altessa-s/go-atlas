// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Idempotency configuration.
const (
	defaultIdempotencyTTL = 24 * time.Hour
)

// Idempotency defines the configuration for idempotency key management.
// Controls how idempotency keys are stored and managed for duplicate request detection.
//
// Example:
//
//	idem := &config.Idempotency{
//		TTL: 24 * time.Hour,
//		Storage: &config.CacheStorageConfig{
//			Type: config.CacheStorageTypeRedis,
//		},
//	}
type Idempotency struct {
	// TTL defines the time-to-live for idempotency keys.
	// Keys are automatically expired after this duration to prevent storage bloat.
	TTL time.Duration `yaml:"ttl" default:"24h"`

	// Storage defines the storage configuration for idempotency keys.
	// Required, specifies backend and settings.
	Storage *CacheStorageConfig `yaml:"storage"`
}

// DefaultIdempotency returns an Idempotency configuration with default values.
// Note: Storage is left as nil since it is a required field.
func DefaultIdempotency() Idempotency {
	return Idempotency{
		TTL: defaultIdempotencyTTL,
	}
}

// Validate performs validation of the idempotency configuration.
// Ensures that TTL and storage are correctly configured.
// Returns an error if validation fails, nil otherwise.
func (i *Idempotency) Validate() error {
	return ValidateStruct(i,
		validation.Field(&i.TTL, validation.Required, ozzo_rules.Duration(), validation.Min(time.Minute)),
		validation.Field(&i.Storage, validation.Required),
	)
}

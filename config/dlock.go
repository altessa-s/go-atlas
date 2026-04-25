// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for DistributionLock configuration.
const (
	defaultDistributionLockNatsBucket = "dlock"
)

// DistributionLockProvider defines the distributed locking provider type.
type DistributionLockProvider string

const (
	// DistributionLockProviderNats represents the NATS JetStream distributed lock provider.
	DistributionLockProviderNats DistributionLockProvider = "nats"
)

// DistributionLockNats defines the NATS-specific configuration for distributed locking.
// Contains settings for NATS JetStream Key-Value bucket creation and management.
type DistributionLockNats struct {
	// Bucket is the name of the NATS JetStream Key-Value bucket
	// where distributed locks will be stored.
	// Defaults to "dlock" if not specified.
	Bucket string `yaml:"bucket" default:"dlock"`
}

// DistributionLock defines the configuration for distributed locking.
// Coordinates operations across multiple service instances using distributed locks.
//
// Example:
//
//	dlock := &config.DistributionLock{
//		Provider: config.DistributionLockProviderNats,
//		Nats:     &config.DistributionLockNats{Bucket: "myapp-locks"},
//	}
type DistributionLock struct {
	// Provider defines the type of distributed locking implementation to use.
	// Must be one of the supported providers (currently only "nats").
	Provider DistributionLockProvider `yaml:"provider"`

	// Nats defines the NATS configuration for distributed locking.
	// Required when Provider is DistributionLockProviderNats, ignored otherwise.
	Nats *DistributionLockNats `yaml:"nats" default:"-"`
}

// DefaultDistributionLockNats returns a DistributionLockNats configuration with default values.
func DefaultDistributionLockNats() DistributionLockNats {
	return DistributionLockNats{
		Bucket: defaultDistributionLockNatsBucket,
	}
}

// DefaultDistributionLock returns a DistributionLock configuration with default values.
// Note: Provider is left as zero value since it is a required field.
func DefaultDistributionLock() DistributionLock {
	return DistributionLock{}
}

// Validate performs validation of the distributed lock configuration.
// Ensures provider is valid and corresponding configuration is provided.
// Returns an error if validation fails, nil otherwise.
func (dl *DistributionLock) Validate() error {
	return ValidateStruct(dl,
		validation.Field(&dl.Provider, validation.Required, ozzo_rules.OneOf(DistributionLockProviderNats)),
		validation.Field(&dl.Nats, validation.When(dl.Provider == DistributionLockProviderNats, validation.NilOrNotEmpty)),
	)
}

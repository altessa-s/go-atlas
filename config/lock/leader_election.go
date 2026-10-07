// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockconfig

import (
	"time"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for LeaderElector configuration.
const (
	defaultLeaderElectorTTL = 10 * time.Second
)

// LeaderElectorProvider defines the type of leader election provider.
type LeaderElectorProvider string

const (
	// LeaderElectorProviderNats represents the NATS leader election provider.
	LeaderElectorProviderNats LeaderElectorProvider = "nats"
)

// LeaderElector defines the configuration for distributed leader election.
// Used to coordinate leadership among multiple service instances.
//
// Example:
//
//	le := &lockconfig.LeaderElector{
//		Provider: lockconfig.LeaderElectorProviderNats,
//		Ttl:      10 * time.Second,
//	}
type LeaderElector struct {
	// Provider specifies which leader election provider to use
	Provider LeaderElectorProvider `yaml:"provider" default:"nats"`
	// Ttl defines the time-to-live for leader election locks
	Ttl time.Duration `yaml:"ttl" default:"10s"`
	// Storage is the storage type of an election bucket the provider creates:
	// memory (the default — the lease is lost on a server restart, forcing a
	// clean re-election) or file, which keeps the bucket and the Fence()
	// sequence across a server restart. An existing bucket keeps its storage.
	Storage storageconfig.KVStorageType `yaml:"storage" default:"memory"`
	// MigrateBucketTTL updates a pre-existing election bucket whose key TTL
	// differs from the provider's instead of failing with
	// ErrBucketTTLMismatch. Off by default: the bucket's key TTL expires the
	// election keys of every process sharing the bucket.
	MigrateBucketTTL bool `yaml:"migrateBucketTTL"`
	// StrictBucketStorage fails with ErrBucketStorageMismatch when the
	// election bucket already exists with another storage type, instead of
	// using it as is with a warning.
	StrictBucketStorage bool `yaml:"strictBucketStorage"`
}

// DefaultLeaderElector returns a LeaderElector configuration with default values.
// Note: Provider is left as zero value since it is a required field.
func DefaultLeaderElector() LeaderElector {
	return LeaderElector{
		Ttl:     defaultLeaderElectorTTL,
		Storage: storageconfig.KVStorageMemory,
	}
}

// Validate performs validation of the LeaderElector configuration.
// Returns an error if validation fails, nil otherwise.
func (le *LeaderElector) Validate() error {
	return validationconfig.ValidateStruct(le,
		validation.Field(&le.Provider, validation.Required, ozzo_rules.OneOf(LeaderElectorProviderNats)),
		validation.Field(&le.Ttl, ozzo_rules.Duration(), validation.Min(time.Second)),
		validation.Field(&le.Storage, validation.In(storageconfig.KVStorageMemory, storageconfig.KVStorageFile)),
	)
}

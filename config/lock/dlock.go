// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockconfig

import (
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for DistributionLock configuration.
const (
	defaultDistributionLockNatsBucket        = "dlock"
	defaultDistributionLockMongodbCollection = "dlocks"
)

// DistributionLockProvider defines the distributed locking provider type.
type DistributionLockProvider string

const (
	// DistributionLockProviderNATS represents the NATS JetStream distributed lock provider.
	DistributionLockProviderNATS DistributionLockProvider = "nats"
	// DistributionLockProviderMongo represents the MongoDB distributed lock provider.
	DistributionLockProviderMongo DistributionLockProvider = "mongo"
)

// DistributionLockMongo defines the MongoDB-specific configuration for
// distributed locking. The *mongo.Database is injected into the factory.
type DistributionLockMongo struct {
	// Collection holds one lease document per lock key.
	// Defaults to "dlocks" if not specified.
	Collection string `yaml:"collection" default:"dlocks"`
}

// DefaultDistributionLockMongo returns a DistributionLockMongo configuration with default values.
func DefaultDistributionLockMongo() DistributionLockMongo {
	return DistributionLockMongo{Collection: defaultDistributionLockMongodbCollection}
}

// DistributionLockNATS defines the NATS-specific configuration for distributed locking.
// Contains settings for NATS JetStream Key-Value bucket creation and management.
type DistributionLockNATS struct {
	// Bucket is the name of the NATS JetStream Key-Value bucket
	// where distributed locks will be stored.
	// Defaults to "dlock" if not specified.
	Bucket string `yaml:"bucket" default:"dlock"`

	// Storage is the storage type of a bucket the provider creates: memory
	// (the default — a lock lives no longer than its TTL) or file, which keeps
	// the bucket and its fencing-token sequence across a server restart. An
	// existing bucket keeps its storage type.
	Storage storageconfig.KVStorageType `yaml:"storage" default:"memory"`

	// MigrateBucketTTL updates a pre-existing bucket whose key TTL differs
	// from the lock TTL instead of failing with ErrBucketTTLMismatch. Off by
	// default: the bucket's key TTL expires every lock in it, including locks
	// of other processes sharing the bucket.
	MigrateBucketTTL bool `yaml:"migrateBucketTTL"`

	// StrictBucketStorage fails with ErrBucketStorageMismatch when the bucket
	// already exists with another storage type, instead of using it as is
	// with a warning.
	StrictBucketStorage bool `yaml:"strictBucketStorage"`
}

// DistributionLock defines the configuration for distributed locking.
// Coordinates operations across multiple service instances using distributed locks.
//
// Example:
//
//	dlock := &lockconfig.DistributionLock{
//		Provider: lockconfig.DistributionLockProviderNATS,
//		NATS:     &lockconfig.DistributionLockNATS{Bucket: "myapp-locks"},
//	}
type DistributionLock struct {
	// Provider defines the type of distributed locking implementation to use.
	// Must be one of the supported providers: "nats" or "mongo".
	Provider DistributionLockProvider `yaml:"provider"`

	// Nats defines the NATS configuration for distributed locking.
	// Required when Provider is DistributionLockProviderNATS, ignored otherwise.
	NATS *DistributionLockNATS `yaml:"nats" default:"-"`

	// Mongodb defines the MongoDB configuration for distributed locking.
	// Required when Provider is DistributionLockProviderMongo, ignored otherwise.
	Mongo *DistributionLockMongo `yaml:"mongo" default:"-"`
}

// DefaultDistributionLockNATS returns a DistributionLockNATS configuration with default values.
func DefaultDistributionLockNATS() DistributionLockNATS {
	return DistributionLockNATS{
		Bucket:  defaultDistributionLockNatsBucket,
		Storage: storageconfig.KVStorageMemory,
	}
}

// Validate performs validation of the NATS distributed lock configuration.
func (n *DistributionLockNATS) Validate() error {
	return validationconfig.ValidateStruct(n,
		validation.Field(&n.Storage, validation.In(storageconfig.KVStorageMemory, storageconfig.KVStorageFile)),
	)
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
	return validationconfig.ValidateStruct(dl,
		validation.Field(&dl.Provider, validation.Required,
			ozzo_rules.OneOf(DistributionLockProviderNATS, DistributionLockProviderMongo)),
		validation.Field(&dl.NATS, validation.When(dl.Provider == DistributionLockProviderNATS, validation.NilOrNotEmpty)),
		validation.Field(&dl.Mongo, validation.When(dl.Provider == DistributionLockProviderMongo, validation.Required)),
	)
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// AuditStorageType defines the type of storage backend for audit events.
type AuditStorageType string

// Supported audit storage types.
const (
	AuditStorageTypeMemory AuditStorageType = "memory"
	AuditStorageTypeMongo  AuditStorageType = "mongo"
)

// Audit defines the configuration for the audit subsystem.
// Controls how audit events are buffered, dispatched, and persisted.
//
// Dispatch-level tunables (buffer, batch, workers, retries, WAL) are
// delegated to the embedded [Dispatch] under the "dispatch" YAML key.
//
// Example:
//
//	audit := &config.Audit{
//		Enabled: true,
//		Storage: AuditStorage{
//			Type: config.AuditStorageTypeMongo,
//			Mongo: &AuditStorageMongo{
//				CollectionName: "audit_events",
//			},
//		},
//		Dispatch: Dispatch{
//			BufferSize:    20000,
//			BatchSize:     200,
//			FlushInterval: 500 * time.Millisecond,
//			Workers:       4,
//			WAL: &WAL{Enabled: true, Dir: "./var/audit/wal"},
//		},
//	}
type Audit struct {
	// Enabled controls whether the auditor is created.
	// When false, the factory returns nil, nil.
	Enabled bool `yaml:"enabled" default:"true"`

	// Storage defines the storage backend for audit events.
	Storage AuditStorage `yaml:"storage"`

	// ShutdownTimeout is the maximum time to wait during graceful shutdown.
	ShutdownTimeout time.Duration `yaml:"shutdownTimeout" default:"30s"`

	// Dispatch holds the async dispatch engine configuration (buffer,
	// batch, workers, retries, WAL).
	Dispatch Dispatch `yaml:"dispatch"`
}

// AuditStorage defines the storage backend configuration for audit events.
type AuditStorage struct {
	// Type selects the storage backend.
	Type AuditStorageType `yaml:"type" default:"memory"`

	// Mongo holds MongoDB-specific storage configuration.
	// Required when Type is "mongo".
	Mongo *AuditStorageMongo `yaml:"mongo"`
}

// AuditStorageMongo holds MongoDB-specific configuration for audit event storage.
type AuditStorageMongo struct {
	// CollectionName is the MongoDB collection name for audit events.
	CollectionName string `yaml:"collectionName" default:"audit_events"`

	// IndexTimeout is the timeout for index creation operations.
	IndexTimeout time.Duration `yaml:"indexTimeout" default:"30s"`

	// TTL is the time-to-live for audit events. Zero means no expiration.
	TTL time.Duration `yaml:"ttl"`
}

// Validate performs validation of the audit configuration.
func (a *Audit) Validate() error {
	return ValidateStruct(a,
		validation.Field(&a.Storage),
	)
}

// Validate performs validation of the audit storage configuration.
func (s *AuditStorage) Validate() error {
	return ValidateStruct(s,
		validation.Field(&s.Type, validation.Required, validation.In(AuditStorageTypeMemory, AuditStorageTypeMongo)),
		validation.Field(&s.Mongo, validation.When(s.Type == AuditStorageTypeMongo, validation.Required)),
	)
}

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
//	}
type Audit struct {
	// Enabled controls whether the auditor is created.
	// When false, the factory returns nil, nil.
	Enabled bool `yaml:"enabled" default:"true"`

	// Storage defines the storage backend for audit events.
	Storage AuditStorage `yaml:"storage"`

	// BufferSize is the event channel buffer capacity.
	BufferSize int `yaml:"bufferSize" default:"10000"`

	// BatchSize is the number of events per storage write.
	BatchSize int `yaml:"batchSize" default:"100"`

	// FlushInterval is the maximum wait before flushing a partial batch.
	FlushInterval time.Duration `yaml:"flushInterval" default:"1s"`

	// Workers is the number of concurrent dispatch goroutines.
	Workers int `yaml:"workers" default:"2"`

	// RetryAttempts is the maximum retries per failed batch.
	RetryAttempts int `yaml:"retryAttempts" default:"3"`

	// RetryBackoff is the base duration for exponential backoff.
	RetryBackoff time.Duration `yaml:"retryBackoff" default:"100ms"`

	// ShutdownTimeout is the maximum time to wait during graceful shutdown.
	ShutdownTimeout time.Duration `yaml:"shutdownTimeout" default:"30s"`

	// BackPressure enables back-pressure mode where Emit blocks when the buffer is full
	// instead of dropping events.
	BackPressure bool `yaml:"backPressure"`

	// WAL configures the optional crash-safe write-ahead log. By default
	// the auditor runs purely in-memory; enable WAL when audit events must
	// survive process crashes (SIGKILL/panic/OOM).
	WAL AuditWAL `yaml:"wal"`
}

// AuditWAL configures the optional WAL backing the auditor.
type AuditWAL struct {
	// Enabled turns on local WAL durability. When false (default) the
	// auditor behaves identically to the previous in-RAM-only dispatcher.
	Enabled bool `yaml:"enabled"`

	// Dir is the directory storing WAL segment files. It is created if missing.
	Dir string `yaml:"dir" default:"./var/audit/wal"`

	// MaxSegmentBytes is the maximum size of a single segment file.
	MaxSegmentBytes int64 `yaml:"maxSegmentBytes" default:"67108864"` // 64 MiB

	// MaxBytes is the soft cap on total bytes across all segments.
	MaxBytes int64 `yaml:"maxBytes" default:"1073741824"` // 1 GiB

	// FsyncInterval is the period between background fsync calls. A
	// shorter interval reduces the loss window after a crash, at the
	// cost of throughput.
	FsyncInterval time.Duration `yaml:"fsyncInterval" default:"5ms"`
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

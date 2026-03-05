// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"fmt"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/audit"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	memorystorage "github.com/altessa-s/go-atlas/data/audit/storages/memory"
	mongostorage "github.com/altessa-s/go-atlas/data/audit/storages/mongo"
)

// ErrDisabled is returned by [AuditorBuilder.Build] when auditing is disabled
// in the configuration. Callers should use [errors.Is] to check for this sentinel.
var ErrDisabled = errors.New("audit: disabled by configuration")

// AuditorBuilder assembles an [audit.Auditor] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [AuditorBuilder.Build] time.
// The builder is not safe for concurrent use.
type AuditorBuilder struct {
	corefactory.Base
	cfg  *config.Audit
	errs []error

	// Dependencies
	mongoDb *mongo.Database
}

// New creates an [AuditorBuilder] for the given audit config.
// Config can be nil — the error surfaces at [AuditorBuilder.Build] time.
func New(cfg *config.Audit) *AuditorBuilder {
	return &AuditorBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the audit Auditor. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
//
// Returns (nil, nil) when the configuration has Enabled set to false.
func (b *AuditorBuilder) Build() (*audit.Auditor, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if !b.cfg.Enabled {
		return nil, ErrDisabled
	}

	storage, err := b.createStorage()
	if err != nil {
		return nil, err
	}

	return b.createAuditor(storage)
}

// createAuditor creates and starts an Auditor with the given storage.
func (b *AuditorBuilder) createAuditor(storage audit.Storage) (*audit.Auditor, error) {
	opts := b.buildOptions()

	auditor, err := audit.New(storage, opts...)
	if err != nil {
		return nil, fmt.Errorf("create auditor: %w", err)
	}

	if err := auditor.Start(); err != nil {
		return nil, fmt.Errorf("start auditor: %w", err)
	}

	return auditor, nil
}

// buildOptions converts config fields into audit.Option values.
func (b *AuditorBuilder) buildOptions() []audit.Option {
	cfg := b.cfg

	opts := []audit.Option{
		audit.WithLogger(b.Logger()),
		audit.WithBufferSize(cfg.BufferSize),
		audit.WithBatchSize(cfg.BatchSize),
		audit.WithWorkers(cfg.Workers),
		audit.WithRetryAttempts(cfg.RetryAttempts),
	}

	if cfg.FlushInterval > 0 {
		opts = append(opts, audit.WithFlushInterval(cfg.FlushInterval))
	}
	if cfg.RetryBackoff > 0 {
		opts = append(opts, audit.WithRetryBackoff(cfg.RetryBackoff))
	}
	if cfg.ShutdownTimeout > 0 {
		opts = append(opts, audit.WithShutdownTimeout(cfg.ShutdownTimeout))
	}
	if cfg.BackPressure {
		opts = append(opts, audit.WithBackPressure())
	}

	return opts
}

// createStorage creates a storage backend based on configuration.
func (b *AuditorBuilder) createStorage() (audit.Storage, error) {
	switch b.cfg.Storage.Type {
	case config.AuditStorageTypeMemory:
		return memorystorage.New(), nil
	case config.AuditStorageTypeMongo:
		return b.createMongoStorage()
	default:
		return nil, b.Errorf("unsupported storage type: %s", b.cfg.Storage.Type)
	}
}

// createMongoStorage creates a MongoDB storage from configuration.
func (b *AuditorBuilder) createMongoStorage() (*mongostorage.Storage, error) {
	if err := b.RequireDependency(b.mongoDb, "mongo database"); err != nil {
		return nil, err
	}

	var opts []mongostorage.Option
	if mongoCfg := b.cfg.Storage.Mongo; mongoCfg != nil {
		opts = append(opts, mongostorage.WithCollectionName(mongoCfg.CollectionName))
		if mongoCfg.IndexTimeout > 0 {
			opts = append(opts, mongostorage.WithIndexTimeout(mongoCfg.IndexTimeout))
		}
		if mongoCfg.TTL > 0 {
			opts = append(opts, mongostorage.WithTtl(mongoCfg.TTL))
		}
	}

	return mongostorage.New(b.mongoDb, opts...)
}

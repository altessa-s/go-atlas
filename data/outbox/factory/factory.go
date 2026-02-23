// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/data/outbox"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	outboxstore "github.com/altessa-s/go-atlas/data/outbox/store/mongo"
)

// Factory creates generic outbox instances from configuration.
type Factory struct {
	corefactory.Base
	scheduler corescheduler.TaskRegistrar
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:      corefactory.NewBase(cfg.logger),
		scheduler: cfg.scheduler,
	}
}

// CreateOutboxWithMongoFromConfig creates a MongoDB-backed outbox for reliable event delivery from configuration.
func (f *Factory) CreateOutboxWithMongoFromConfig(
	cfg *config.Outbox, db *mongo.Database, handler outbox.Handler,
) (*outbox.Outbox, error) {
	if err := f.RequireDependency(db, "MongoDB database"); err != nil {
		return nil, err
	}

	store, err := outboxstore.New(db)
	if err != nil {
		return nil, f.WrapError(err, "failed to create outbox store")
	}

	return f.createOutboxWithStore(cfg, store, handler)
}

// CreateOutboxWithMongoCollectionFromConfig creates a MongoDB-backed outbox using an existing collection from configuration.
func (f *Factory) CreateOutboxWithMongoCollectionFromConfig(
	cfg *config.Outbox, col *mongo.Collection, handler outbox.Handler,
) (*outbox.Outbox, error) {
	if err := f.RequireDependency(col, "MongoDB collection"); err != nil {
		return nil, err
	}

	store, err := outboxstore.NewWithCollectionOptions(col)
	if err != nil {
		return nil, f.WrapError(err, "failed to create outbox store")
	}

	return f.createOutboxWithStore(cfg, store, handler)
}

// createOutboxWithStore creates an outbox with the given store (shared logic).
func (f *Factory) createOutboxWithStore(cfg *config.Outbox, store outbox.Store, handler outbox.Handler) (*outbox.Outbox, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if !cfg.Enabled {
		return nil, nil //nolint:nilnil
	}

	opts := f.buildOutboxOptions(cfg)

	// Add scheduler and schedule options if scheduler is available
	if f.scheduler != nil {
		opts = append(opts, outbox.WithScheduler(f.scheduler))
		if cfg.DispatchSchedule != "" {
			opts = append(opts, outbox.WithDispatchSchedule(cfg.DispatchSchedule))
		}
		if cfg.UnlockSchedule != "" {
			opts = append(opts, outbox.WithUnlockSchedule(cfg.UnlockSchedule))
		}
		if cfg.CleanupSchedule != "" {
			opts = append(opts, outbox.WithCleanupSchedule(cfg.CleanupSchedule))
		}
	}

	return outbox.New(store, handler, opts...), nil
}

// buildOutboxOptions builds outbox options from configuration.
func (f *Factory) buildOutboxOptions(cfg *config.Outbox) []outbox.Option {
	opts := []outbox.Option{
		outbox.WithLogger(f.Logger()),
		outbox.WithFetchTimeout(cfg.FetchTimeout),
		outbox.WithHandleTimeout(cfg.HandleTimeout),
		outbox.WithUpdateTimeout(cfg.UpdateTimeout),
		outbox.WithEventsBatchSize(cfg.MessagesBatchSize),
		outbox.WithRetryMaxAttempts(cfg.RetryMaxAttempts),
		outbox.WithPublishedEventsLifetime(cfg.PublishedEventsLifetime),
	}
	opts = slices.AppendIf(opts, cfg.TopicCompaction, outbox.WithCompaction())

	return opts
}

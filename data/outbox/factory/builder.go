// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/data/outbox"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	outboxstore "github.com/altessa-s/go-atlas/data/outbox/store/mongo"
)

// OutboxBuilder assembles an [outbox.Outbox] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at build time.
// The builder is not safe for concurrent use.
type OutboxBuilder struct {
	corefactory.Base
	cfg  *config.Outbox
	errs []error

	// Dependencies
	scheduler corescheduler.TaskRegistrar
}

// New creates a new [OutboxBuilder] for the given outbox config.
// Config can be nil -- the error surfaces at build time.
func New(cfg *config.Outbox) *OutboxBuilder {
	return &OutboxBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// BuildWithMongoDB creates a MongoDB-backed outbox for reliable event delivery.
// The database and handler are required parameters.
func (b *OutboxBuilder) BuildWithMongoDB(db *mongo.Database, handler outbox.Handler) (*outbox.Outbox, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if err := b.RequireDependency(db, "MongoDB database"); err != nil {
		return nil, err
	}

	store, err := outboxstore.New(db)
	if err != nil {
		return nil, b.WrapError(err, "failed to create outbox store")
	}

	return b.createOutboxWithStore(store, handler)
}

// BuildWithMongoCollection creates a MongoDB-backed outbox using an existing collection.
// The collection and handler are required parameters.
func (b *OutboxBuilder) BuildWithMongoCollection(col *mongo.Collection, handler outbox.Handler) (*outbox.Outbox, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if err := b.RequireDependency(col, "MongoDB collection"); err != nil {
		return nil, err
	}

	store, err := outboxstore.NewWithCollectionOptions(col)
	if err != nil {
		return nil, b.WrapError(err, "failed to create outbox store")
	}

	return b.createOutboxWithStore(store, handler)
}

// createOutboxWithStore creates an outbox with the given store (shared logic).
func (b *OutboxBuilder) createOutboxWithStore(store outbox.Store, handler outbox.Handler) (*outbox.Outbox, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if !b.cfg.Enabled {
		return nil, nil //nolint:nilnil
	}

	opts := b.buildOutboxOptions()

	// Add scheduler and schedule options if scheduler is available
	if b.scheduler != nil {
		opts = append(opts, outbox.WithScheduler(b.scheduler))
		if b.cfg.DispatchSchedule != "" {
			opts = append(opts, outbox.WithDispatchSchedule(b.cfg.DispatchSchedule))
		}
		if b.cfg.UnlockSchedule != "" {
			opts = append(opts, outbox.WithUnlockSchedule(b.cfg.UnlockSchedule))
		}
		if b.cfg.CleanupSchedule != "" {
			opts = append(opts, outbox.WithCleanupSchedule(b.cfg.CleanupSchedule))
		}
	}

	return outbox.New(store, handler, opts...), nil
}

// buildOutboxOptions builds outbox options from configuration.
func (b *OutboxBuilder) buildOutboxOptions() []outbox.Option {
	opts := []outbox.Option{
		outbox.WithLogger(b.Logger()),
		outbox.WithFetchTimeout(b.cfg.FetchTimeout),
		outbox.WithHandleTimeout(b.cfg.HandleTimeout),
		outbox.WithUpdateTimeout(b.cfg.UpdateTimeout),
		outbox.WithEventsBatchSize(b.cfg.MessagesBatchSize),
		outbox.WithRetryMaxAttempts(b.cfg.RetryMaxAttempts),
		outbox.WithPublishedEventsLifetime(b.cfg.PublishedEventsLifetime),
	}
	opts = slices.AppendIf(opts, b.cfg.TopicCompaction, outbox.WithCompaction())

	return opts
}

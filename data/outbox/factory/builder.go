// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/data/outbox"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	outboxmongo "github.com/altessa-s/go-atlas/data/outbox/storages/mongo"
	outboxsql "github.com/altessa-s/go-atlas/data/outbox/storages/sqldb"
)

// OutboxBuilder assembles an [outbox.Outbox] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at build time.
// The builder is not safe for concurrent use.
type OutboxBuilder struct {
	corefactory.Base
	cfg  *brokerconfig.Outbox
	errs []error

	// Dependencies
	scheduler corescheduler.TaskRegistrar
}

// New creates a new [OutboxBuilder] for the given outbox config.
// Config can be nil -- the error surfaces at build time.
func New(cfg *brokerconfig.Outbox) *OutboxBuilder {
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

	store, err := outboxmongo.New(db)
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

	store, err := outboxmongo.NewWithCollectionOptions(col)
	if err != nil {
		return nil, b.WrapError(err, "failed to create outbox store")
	}

	return b.createOutboxWithStore(store, handler)
}

// BuildWithSQLDB creates a SQL-backed outbox (PostgreSQL, MySQL or MariaDB,
// selected by dialect) for reliable event delivery. The database handle and
// handler are required. With the config's EnsureSchema set, the events table
// is created if it does not exist; otherwise it must already exist (the
// store's EnsureSchema or migrations). Save must run on the business
// transaction: pass outboxsql.WithTx(ctx, tx).
func (b *OutboxBuilder) BuildWithSQLDB(db *sql.DB, dialect outboxsql.Dialect, handler outbox.Handler) (*outbox.Outbox, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if err := b.RequireDependency(db, "SQL database"); err != nil {
		return nil, err
	}

	store, err := outboxsql.New(db, dialect)
	if err != nil {
		return nil, b.WrapError(err, "failed to create outbox store")
	}
	if b.cfg != nil && b.cfg.Enabled && b.cfg.EnsureSchema {
		ctx, cancel := context.WithTimeout(context.Background(), ensureSchemaTimeout)
		defer cancel()
		if err := store.EnsureSchema(ctx); err != nil {
			return nil, b.WrapError(err, "ensure outbox schema")
		}
	}

	return b.createOutboxWithStore(store, handler)
}

// ensureSchemaTimeout bounds the schema creation BuildWithSQLDB runs when the
// config asks for it, so a lock wait cannot stall startup indefinitely.
const ensureSchemaTimeout = 30 * time.Second

// createOutboxWithStore creates an outbox with the given store (shared logic).
func (b *OutboxBuilder) createOutboxWithStore(store outbox.Storage, handler outbox.Handler) (*outbox.Outbox, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if !b.cfg.Enabled {
		return nil, nil //nolint:nilnil
	}

	opts := b.buildOutboxOptions()

	// Add scheduler and schedule options if scheduler is available.
	// Task IDs go on every time (not gated by the schedule strings)
	// because the runtime collision check inspects all four IDs even
	// when some schedules are empty — keeping the IDs in lockstep with
	// their YAML counterparts means a config change that flips a
	// schedule on inherits the operator's overrides automatically.
	if b.scheduler != nil {
		opts = append(opts,
			outbox.WithScheduler(b.scheduler),
			outbox.WithDispatchTaskID(b.cfg.DispatchTaskID),
			outbox.WithUnlockTaskID(b.cfg.UnlockTaskID),
			outbox.WithExpireTaskID(b.cfg.ExpireTaskID),
			outbox.WithCleanupTaskID(b.cfg.CleanupTaskID),
			outbox.WithStatsTaskID(b.cfg.StatsTaskID),
		)
		opts = slices.AppendIf(opts, b.cfg.DispatchSchedule != "", outbox.WithDispatchSchedule(b.cfg.DispatchSchedule))
		opts = slices.AppendIf(opts, b.cfg.UnlockSchedule != "", outbox.WithUnlockSchedule(b.cfg.UnlockSchedule))
		opts = slices.AppendIf(opts, b.cfg.CleanupSchedule != "", outbox.WithCleanupSchedule(b.cfg.CleanupSchedule))
		opts = slices.AppendIf(opts, b.cfg.ExpireSchedule != "", outbox.WithExpireSchedule(b.cfg.ExpireSchedule))
		opts = slices.AppendIf(opts, b.cfg.StatsSchedule != "", outbox.WithStatsSchedule(b.cfg.StatsSchedule))
	}

	ob := outbox.New(store, handler, opts...)
	if err := ob.RegisterTasks(context.Background()); err != nil {
		return ob, b.WrapError(err, "register outbox tasks")
	}
	return ob, nil
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
		outbox.WithRetryBaseDelay(b.cfg.RetryBaseDelay),
		outbox.WithRetryMaxDelay(b.cfg.RetryMaxDelay),
		outbox.WithMaxLockTime(b.cfg.MaxLockTime),
		outbox.WithMaxPayloadBytes(b.cfg.MaxPayloadBytes),
		outbox.WithPublishedEventsLifetime(b.cfg.PublishedEventsLifetime),
		outbox.WithDefaultEventTTL(b.cfg.DefaultEventTTL),
	}
	opts = slices.AppendIf(opts, b.cfg.TopicCompaction, outbox.WithCompaction())

	return opts
}

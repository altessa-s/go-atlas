// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/inprogress"
	"github.com/altessa-s/go-atlas/transport/broker/outbox"
	"github.com/altessa-s/go-atlas/transport/broker/providers/nats/recovery"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	outboxstore "github.com/altessa-s/go-atlas/data/outbox/store/mongo"
	natsprovider "github.com/altessa-s/go-atlas/transport/broker/providers/nats"
)

// BrokerBuilder assembles a [broker.Broker] and related components step by step
// using a fluent API. Create instances with [New]. Errors are accumulated and
// reported at [BrokerBuilder.Build] time. The builder is not safe for concurrent use.
type BrokerBuilder struct {
	corefactory.Base
	cfg  *config.Broker
	errs []error

	// Dependencies
	publishConverter broker.PublishConverter
	scheduler        corescheduler.TaskRegistrar
}

// New creates a new [BrokerBuilder] for the given broker config.
// Config can be nil -- the error surfaces at [BrokerBuilder.Build] time.
func New(cfg *config.Broker) *BrokerBuilder {
	return &BrokerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the broker with the given provider. Errors from fluent
// methods are accumulated and reported here via [errors.Join].
func (b *BrokerBuilder) Build(provider broker.Provider) (*broker.Broker, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if err := b.RequireDependency(provider, "broker provider"); err != nil {
		return nil, err
	}

	opts := b.applyBrokerDefaults()
	return broker.New(provider, opts...), nil
}

// CreateInProgressManager creates a new InProgress heartbeat manager from the
// builder's broker configuration. When a scheduler is configured, the builder
// automatically registers RunTickCycle task for periodic heartbeat execution.
func (b *BrokerBuilder) CreateInProgressManager() (*inprogress.Manager, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	cfg := &b.cfg.InProgress
	if !cfg.Enabled {
		return nil, nil //nolint:nilnil
	}

	opts := make([]inprogress.Option, 0, 4)
	opts = append(opts, inprogress.WithLogger(b.Logger()))
	if b.scheduler != nil {
		opts = append(opts, inprogress.WithScheduler(b.scheduler))
		opts = append(opts, inprogress.WithTickSchedule(cfg.TickSchedule))
	}

	return inprogress.New(opts...), nil
}

// CreateOutboxWithMongoDB creates a MongoDB-backed outbox for reliable message
// delivery using the builder's broker configuration.
func (b *BrokerBuilder) CreateOutboxWithMongoDB(
	db *mongo.Database, publisher outbox.Publisher,
) (*outbox.Outbox, error) {
	if err := b.RequireDependency(db, "MongoDB database"); err != nil {
		return nil, err
	}

	store, err := outboxstore.New(db)
	if err != nil {
		return nil, b.WrapError(err, "failed to create outbox store")
	}

	return b.createOutboxWithStore(store, publisher)
}

// CreateOutboxWithMongoCollection creates a MongoDB-backed outbox using an
// existing collection and the builder's broker configuration.
func (b *BrokerBuilder) CreateOutboxWithMongoCollection(
	col *mongo.Collection, publisher outbox.Publisher,
) (*outbox.Outbox, error) {
	if err := b.RequireDependency(col, "MongoDB collection"); err != nil {
		return nil, err
	}

	store, err := outboxstore.NewWithCollectionOptions(col)
	if err != nil {
		return nil, b.WrapError(err, "failed to create outbox store")
	}

	return b.createOutboxWithStore(store, publisher)
}

// NatsProviderWithRecovery bundles a [natsprovider.Nats] provider with its
// optional [recovery.Manager]. Call [NatsProviderWithRecovery.Close] when the
// result is no longer needed to stop the recovery manager.
type NatsProviderWithRecovery struct {
	// Provider is the NATS broker provider.
	Provider *natsprovider.Nats
	// Recovery is the optional recovery manager (nil if recovery is disabled).
	Recovery *recovery.Manager
}

// Close stops the recovery manager if it was started.
// The NATS provider should be closed separately via its connection.
func (n *NatsProviderWithRecovery) Close() {
	if n.Recovery != nil {
		n.Recovery.Close()
	}
}

// CreateNatsProviderWithRecovery creates a NATS provider with optional recovery manager
// using the builder's broker configuration. If recovery is enabled, the recovery manager
// is created and will register background tasks with the scheduler if one is provided.
//
// The recovery manager is NOT started automatically. Call Recovery.Start() after
// registering streams to begin advisory event listening.
//
// Example:
//
//	result, err := b.CreateNatsProviderWithRecovery(natsConn)
//	if err != nil {
//	    return err
//	}
//	defer result.Close()
//
//	// Register streams for recovery before starting
//	if result.Recovery != nil {
//	    result.Recovery.RegisterStream(streamCfg)
//	    result.Recovery.Start()
//	}
func (b *BrokerBuilder) CreateNatsProviderWithRecovery(conn *nats.Conn) (*NatsProviderWithRecovery, error) {
	provider, err := b.createNatsProvider(conn)
	if err != nil {
		return nil, err
	}

	recoveryMgr, err := b.CreateRecoveryManager(provider)
	if err != nil {
		return nil, err
	}

	return &NatsProviderWithRecovery{
		Provider: provider,
		Recovery: recoveryMgr,
	}, nil
}

// CreateRecoveryManager creates a NATS JetStream recovery manager from the
// builder's broker configuration. The recovery manager automatically recovers
// deleted streams and consumers.
//
// If [config.Broker.Nats] is nil, or recovery is not enabled, returns nil, nil.
//
// The manager will register recovery tasks (health check, stale cleanup) if scheduler
// is provided via builder options.
func (b *BrokerBuilder) CreateRecoveryManager(provider *natsprovider.Nats) (*recovery.Manager, error) {
	cfg := b.cfg.Nats
	if cfg == nil || cfg.Recovery == nil || !cfg.Recovery.Enabled {
		return nil, nil //nolint:nilnil
	}

	if err := b.RequireDependency(provider, "NATS provider"); err != nil {
		return nil, err
	}

	recoveryCfg := cfg.Recovery

	// Build options from configuration.
	opts := []recovery.Option{
		recovery.WithLogger(b.Logger()),
		recovery.WithMaxRecoveryAttempts(recoveryCfg.MaxRecoveryAttempts),
		recovery.WithRecoveryBackoff(recoveryCfg.RecoveryBackoff),
		recovery.WithStaleRecoveryTimeout(recoveryCfg.StaleRecoveryTimeout),
	}

	// Add scheduler and schedule options if scheduler is available
	if b.scheduler != nil {
		opts = append(opts, recovery.WithScheduler(b.scheduler))
		opts = append(opts, recovery.WithHealthCheckSchedule(recoveryCfg.HealthCheckSchedule))
		opts = append(opts, recovery.WithStaleRecoveryCleanupSchedule(recoveryCfg.StaleRecoveryCleanupSchedule))
	}

	// Create recovery manager.
	manager, err := recovery.New(provider, opts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create recovery manager")
	}

	return manager, nil
}

// createOutboxWithStore creates an outbox with the given store (shared logic).
func (b *BrokerBuilder) createOutboxWithStore(store outbox.Store, publisher outbox.Publisher) (*outbox.Outbox, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	cfg := &b.cfg.Outbox
	if !cfg.Enabled {
		return nil, nil //nolint:nilnil
	}

	if err := b.RequireDependency(publisher, "outbox publisher"); err != nil {
		return nil, err
	}

	opts := b.buildOutboxOptions()

	// Add scheduler and schedule options if scheduler is available
	if b.scheduler != nil {
		opts = append(opts, outbox.WithScheduler(b.scheduler))
		if cfg.DispatchSchedule != "" {
			opts = append(opts, outbox.WithDispatchSchedule(cfg.DispatchSchedule))
		}
		if cfg.UnlockSchedule != "" {
			opts = append(opts, outbox.WithUnlockSchedule(cfg.UnlockSchedule))
		}
		if cfg.CleanupSchedule != "" {
			opts = append(opts, outbox.WithCleanupSchedule(cfg.CleanupSchedule))
		}
		if cfg.ExpireSchedule != "" {
			opts = append(opts, outbox.WithExpireSchedule(cfg.ExpireSchedule))
		}
	}

	return outbox.New(store, publisher, opts...), nil
}

// buildOutboxOptions builds outbox options from the builder's broker configuration.
func (b *BrokerBuilder) buildOutboxOptions() []outbox.Option {
	cfg := &b.cfg.Outbox
	opts := []outbox.Option{
		outbox.WithLogger(b.Logger()),
		outbox.WithFetchTimeout(cfg.FetchTimeout),
		outbox.WithHandleTimeout(cfg.HandleTimeout),
		outbox.WithUpdateTimeout(cfg.UpdateTimeout),
		outbox.WithMessagesBatchSize(cfg.MessagesBatchSize),
		outbox.WithRetryMaxAttempts(cfg.RetryMaxAttempts),
		outbox.WithPublishedEventsLifetime(cfg.PublishedEventsLifetime),
		outbox.WithDefaultEventTTL(cfg.DefaultEventTTL),
	}
	opts = slices.AppendIf(opts, cfg.TopicCompaction, outbox.WithTopicCompaction())

	return opts
}

// applyBrokerDefaults returns factory default broker options.
func (b *BrokerBuilder) applyBrokerDefaults() []broker.Option {
	opts := []broker.Option{broker.WithLogger(b.Logger())}
	opts = slices.AppendIf(opts, b.publishConverter != nil, broker.WithPublishConverter(b.publishConverter))
	return opts
}

// createNatsProvider creates a NATS broker provider from an existing connection.
func (b *BrokerBuilder) createNatsProvider(conn *nats.Conn) (*natsprovider.Nats, error) {
	if err := b.RequireDependency(conn, "NATS connection"); err != nil {
		return nil, err
	}

	provider, err := natsprovider.New(conn)
	if err != nil {
		return nil, b.WrapError(err, "failed to create NATS provider")
	}

	return provider, nil
}

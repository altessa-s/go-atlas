// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"

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

// Factory creates message brokers and related components from [config]
// configuration structures. Use [New] to construct a factory.
type Factory struct {
	corefactory.Base
	publishConverter broker.PublishConverter
	scheduler        corescheduler.TaskRegistrar
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:             corefactory.NewBase(cfg.logger),
		publishConverter: cfg.publishConverter,
		scheduler:        cfg.scheduler,
	}
}

// CreateBrokerFromConfig creates a Message Broker from configuration.
func (f *Factory) CreateBrokerFromConfig(cfg *config.Broker, provider broker.Provider) (*broker.Broker, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if err := f.RequireDependency(provider, "broker provider"); err != nil {
		return nil, err
	}

	opts := f.applyBrokerDefaults()
	return broker.New(provider, opts...), nil
}

// CreateInProgressManagerFromConfig creates a new InProgress heartbeat manager from configuration.
// When a scheduler is configured, the factory automatically registers RunTickCycle task
// for periodic heartbeat execution.
func (f *Factory) CreateInProgressManagerFromConfig(cfg *config.InProgress) (*inprogress.Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if !cfg.Enabled {
		return nil, nil //nolint:nilnil
	}

	opts := []inprogress.Option{
		inprogress.WithLogger(f.Logger()),
	}

	// Add scheduler and tick schedule options if scheduler is available
	if f.scheduler != nil {
		opts = append(opts, inprogress.WithScheduler(f.scheduler))
		opts = append(opts, inprogress.WithTickSchedule(cfg.TickSchedule))
	}

	return inprogress.New(opts...), nil
}

// CreateOutboxWithMongoFromConfig creates a MongoDB-backed outbox for reliable message delivery from configuration.
func (f *Factory) CreateOutboxWithMongoFromConfig(
	cfg *config.Outbox, db *mongo.Database, publisher outbox.Publisher,
) (*outbox.Outbox, error) {
	if err := f.RequireDependency(db, "MongoDB database"); err != nil {
		return nil, err
	}

	store, err := outboxstore.New(db)
	if err != nil {
		return nil, f.WrapError(err, "failed to create outbox store")
	}

	return f.createOutboxWithStore(cfg, store, publisher)
}

// CreateOutboxWithMongoCollectionFromConfig creates a MongoDB-backed outbox using an existing collection from configuration.
func (f *Factory) CreateOutboxWithMongoCollectionFromConfig(
	cfg *config.Outbox, col *mongo.Collection, publisher outbox.Publisher,
) (*outbox.Outbox, error) {
	if err := f.RequireDependency(col, "MongoDB collection"); err != nil {
		return nil, err
	}

	store, err := outboxstore.NewWithCollectionOptions(col)
	if err != nil {
		return nil, f.WrapError(err, "failed to create outbox store")
	}

	return f.createOutboxWithStore(cfg, store, publisher)
}

// createOutboxWithStore creates an outbox with the given store (shared logic).
func (f *Factory) createOutboxWithStore(cfg *config.Outbox, store outbox.Store, publisher outbox.Publisher) (*outbox.Outbox, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if !cfg.Enabled {
		return nil, nil //nolint:nilnil
	}

	if err := f.RequireDependency(publisher, "outbox publisher"); err != nil {
		return nil, err
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

	return outbox.New(store, publisher, opts...), nil
}

// buildOutboxOptions builds outbox options from configuration.
func (f *Factory) buildOutboxOptions(cfg *config.Outbox) []outbox.Option {
	opts := []outbox.Option{
		outbox.WithLogger(f.Logger()),
		outbox.WithFetchTimeout(cfg.FetchTimeout),
		outbox.WithHandleTimeout(cfg.HandleTimeout),
		outbox.WithUpdateTimeout(cfg.UpdateTimeout),
		outbox.WithMessagesBatchSize(cfg.MessagesBatchSize),
		outbox.WithRetryMaxAttempts(cfg.RetryMaxAttempts),
		outbox.WithPublishedEventsLifetime(cfg.PublishedEventsLifetime),
	}
	opts = slices.AppendIf(opts, cfg.TopicCompaction, outbox.WithTopicCompaction())

	return opts
}

// applyBrokerDefaults returns factory default broker options.
func (f *Factory) applyBrokerDefaults() []broker.Option {
	opts := []broker.Option{broker.WithLogger(f.Logger())}
	opts = slices.AppendIf(opts, f.publishConverter != nil, broker.WithPublishConverter(f.publishConverter))
	return opts
}

// createNatsProvider creates a NATS broker provider from an existing connection.
func (f *Factory) createNatsProvider(conn *nats.Conn) (*natsprovider.Nats, error) {
	if err := f.RequireDependency(conn, "NATS connection"); err != nil {
		return nil, err
	}

	provider, err := natsprovider.New(conn)
	if err != nil {
		return nil, f.WrapError(err, "failed to create NATS provider")
	}

	return provider, nil
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

// CreateNatsProviderWithRecoveryFromConfig creates a NATS provider with optional recovery manager.
// If recovery is enabled in configuration, the recovery manager is created and will
// register background tasks with the scheduler if scheduler is provided via factory options.
//
// The recovery manager is NOT started automatically. Call Recovery.Start() after
// registering streams to begin advisory event listening.
//
// Example:
//
//	result, err := f.CreateNatsProviderWithRecoveryFromConfig(&cfg.Nats, natsConn)
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
func (f *Factory) CreateNatsProviderWithRecoveryFromConfig(cfg *config.Nats, conn *nats.Conn) (*NatsProviderWithRecovery, error) {
	provider, err := f.createNatsProvider(conn)
	if err != nil {
		return nil, err
	}

	recoveryMgr, err := f.CreateRecoveryManagerFromConfig(cfg, provider)
	if err != nil {
		return nil, err
	}

	return &NatsProviderWithRecovery{
		Provider: provider,
		Recovery: recoveryMgr,
	}, nil
}

// CreateRecoveryManagerFromConfig creates a NATS JetStream recovery manager from configuration.
// The recovery manager automatically recovers deleted streams and consumers.
//
// If cfg.Recovery is nil or Enabled is false, returns nil, nil.
//
// The manager will register recovery tasks (health check, stale cleanup) if scheduler
// is provided via factory options.
func (f *Factory) CreateRecoveryManagerFromConfig(cfg *config.Nats, provider *natsprovider.Nats) (*recovery.Manager, error) {
	if cfg == nil || cfg.Recovery == nil || !cfg.Recovery.Enabled {
		return nil, nil //nolint:nilnil
	}

	if err := f.RequireDependency(provider, "NATS provider"); err != nil {
		return nil, err
	}

	recoveryCfg := cfg.Recovery

	// Build options from configuration.
	opts := []recovery.Option{
		recovery.WithLogger(f.Logger()),
		recovery.WithMaxRecoveryAttempts(recoveryCfg.MaxRecoveryAttempts),
		recovery.WithRecoveryBackoff(recoveryCfg.RecoveryBackoff),
		recovery.WithStaleRecoveryTimeout(recoveryCfg.StaleRecoveryTimeout),
	}

	// Add scheduler and schedule options if scheduler is available
	if f.scheduler != nil {
		opts = append(opts, recovery.WithScheduler(f.scheduler))
		opts = append(opts, recovery.WithHealthCheckSchedule(recoveryCfg.HealthCheckSchedule))
		opts = append(opts, recovery.WithStaleRecoveryCleanupSchedule(recoveryCfg.StaleRecoveryCleanupSchedule))
	}

	// Create recovery manager.
	manager, err := recovery.New(provider, opts...)
	if err != nil {
		return nil, f.WrapError(err, "failed to create recovery manager")
	}

	return manager, nil
}

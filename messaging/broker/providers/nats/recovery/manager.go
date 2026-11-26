// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"cmp"
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas/messaging/broker"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
	schedtask "github.com/altessa-s/go-horai/task"
)

// Manager orchestrates automatic recovery for JetStream streams and consumers.
// It manages the lifecycle of streams, consumers, and subscriptions, automatically
// recovering them when deletion is detected via [AdvisoryListener] or [HealthMonitor].
//
// Background tasks (health checks, stale recovery cleanup) are designed to be run
// via an external scheduler. Use [Manager.RegisterHealthCheckSchedulerFunc]
// and [Manager.RegisterStaleCleanupSchedulerFunc] with [schedtask.Config].
//
// All exported methods are safe for concurrent use.
type Manager struct {
	provider *natsprovider.NATS
	logger   *slog.Logger

	registry   *Registry
	supervisor *Supervisor
	advisory   *AdvisoryListener
	healthMon  *HealthMonitor
	timeoutMon *RecoveryTimeoutMonitor

	subscriptions   []*ManagedSubscription
	subscriptionsMu sync.RWMutex

	closed atomic.Bool

	scheduler        schedtask.Registrar
	healthCheckTask  *cycle.Guard // Guards RunHealthCheckCycle and marks scheduler management.
	staleCleanupTask *cycle.Guard // Guards RunStaleRecoveryCleanup and marks scheduler management.
}

// New creates a new Manager with the given NATS provider and options.
//
// Example:
//
//	provider, _ := natsprovider.New(natsConn)
//	manager, _ := recovery.New(provider,
//	    recovery.WithMaxRecoveryAttempts(3),
//	    recovery.WithStaleRecoveryTimeout(20 * time.Minute),
//	)
func New(provider *natsprovider.NATS, opts ...Option) (*Manager, error) {
	if provider == nil {
		return nil, ErrNilJetStream
	}

	cfg := newOptions(opts...)

	m := &Manager{
		provider:  provider,
		logger:    cmp.Or(cfg.logger, slog.New(slog.DiscardHandler)),
		scheduler: cfg.scheduler,
	}
	m.healthCheckTask = cycle.New(m.runHealthCheckCycleInternal)
	m.staleCleanupTask = cycle.New(m.runStaleRecoveryCleanupInternal)

	js := provider.JetStream()
	nc := provider.NATSConn()

	// Initialize components.
	m.registry = NewRegistry()

	m.supervisor = NewSupervisor(js, m.registry,
		WithSupervisorLogger(m.logger),
		WithSupervisorMaxAttempts(cfg.maxRecoveryAttempts),
		WithSupervisorBackoff(cfg.recoveryBackoff),
		WithSupervisorOnRecoverySuccess(cfg.onRecoverySuccess),
		WithSupervisorOnRecoveryFailure(cfg.onRecoveryFailure),
		WithSupervisorOnManualNeeded(cfg.onManualRecoveryNeeded),
		WithSupervisorOnStaleCleared(cfg.onStaleRecoveryCleared),
	)

	m.advisory = NewAdvisoryListener(nc, m.registry, m.supervisor,
		WithAdvisoryListenerLogger(m.logger),
	)

	m.healthMon = NewHealthMonitor(js, m.registry, m.supervisor,
		WithHealthMonitorLogger(m.logger),
	)

	m.timeoutMon = NewRecoveryTimeoutMonitor(m.supervisor,
		WithRecoveryTimeoutMonitorLogger(m.logger),
		WithRecoveryTimeoutMonitorTimeout(cfg.staleRecoveryTimeout),
	)

	// Register recovery tasks with scheduler if provided
	if err := m.registerTasks(cfg); err != nil {
		return nil, coreerrs.WrapOperation(err, "register recovery tasks")
	}

	return m, nil
}

// Start begins the advisory listener for instant deletion detection.
// Background tasks (health checks, stale cleanup) should be registered
// with an external scheduler using RunHealthCheckCycle and RunStaleRecoveryCleanup.
//
// Call Close to stop the manager.
func (m *Manager) Start() error {
	if m.closed.Load() {
		return ErrManagerClosed
	}

	// Start advisory listener for instant detection.
	if err := m.advisory.Start(); err != nil {
		return coreerrs.WrapOperation(err, "start advisory listener")
	}

	m.logger.Info("recovery manager started")
	return nil
}

// Close stops the manager and all its components.
// Active subscriptions are drained before closing.
func (m *Manager) Close() {
	if m.closed.Swap(true) {
		return // Already closed.
	}

	// Stop advisory listener.
	_ = m.advisory.Stop() //nolint:errcheck // Best-effort cleanup during shutdown

	// Stop supervisor.
	m.supervisor.Close()

	// Drain all subscriptions.
	m.subscriptionsMu.Lock()
	subs := m.subscriptions
	m.subscriptions = nil
	m.subscriptionsMu.Unlock()

	for _, sub := range subs {
		sub.Unsubscribe()
	}

	m.logger.Info("recovery manager closed")
}

// RegisterStream registers a stream configuration for recovery.
// The stream will be automatically recovered if it is deleted.
//
// Example:
//
//	manager.RegisterStream(jetstream.StreamConfig{
//	    Name:     "ORDERS",
//	    Subjects: []string{"orders.>"},
//	}, recovery.WithRecoveryStrategy(recovery.RecoveryStrategyAuto))
func (m *Manager) RegisterStream(cfg jetstream.StreamConfig, opts ...StreamOption) error {
	if m.closed.Load() {
		return ErrManagerClosed
	}

	if cfg.Name == "" {
		return ErrInvalidStreamConfig
	}

	streamOpts := newStreamOptions(opts...)

	m.registry.RegisterStream(cfg.Name, cfg, streamOpts.strategy)

	m.logger.Info("stream registered for recovery",
		slog.String("stream", cfg.Name),
		slog.String("strategy", streamOpts.strategy.String()))

	return nil
}

// UnregisterStream removes a stream from recovery management.
// The stream and its consumers will no longer be recovered if deleted.
func (m *Manager) UnregisterStream(name string) {
	m.registry.UnregisterStream(name)

	m.logger.Info("stream unregistered from recovery",
		slog.String("stream", name))
}

// CreateStream creates or updates a stream and registers it for recovery.
//
// Example:
//
//	stream, err := manager.CreateStream(ctx, jetstream.StreamConfig{
//	    Name:     "ORDERS",
//	    Subjects: []string{"orders.>"},
//	})
func (m *Manager) CreateStream(ctx context.Context, cfg jetstream.StreamConfig, opts ...StreamOption) (jetstream.Stream, error) {
	if m.closed.Load() {
		return nil, ErrManagerClosed
	}

	// Register for recovery first.
	if err := m.RegisterStream(cfg, opts...); err != nil {
		return nil, err
	}

	// Create the stream.
	stream, err := m.provider.JetStream().CreateOrUpdateStream(ctx, cfg)
	if err != nil {
		// Unregister on failure.
		m.registry.UnregisterStream(cfg.Name)
		return nil, err
	}

	return stream, nil
}

// Subscribe creates a managed subscription with automatic recovery.
// When the consumer is deleted, it will be automatically recreated
// and the subscription re-established.
//
// The factory parameter should be created using natsprovider.SubscriberWithConsumer
// or similar factory functions. The consumerName is required for recovery tracking.
//
// Example:
//
//	sub, err := manager.Subscribe(ctx, "ORDERS", "order-processor", handler,
//	    natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
//	        Durable: "order-processor",
//	    }),
//	)
func (m *Manager) Subscribe(
	ctx context.Context,
	stream string,
	consumerName string,
	handler broker.SubscriberHandler,
	factory natsprovider.SubscriberFactory,
) (*ManagedSubscription, error) {
	if m.closed.Load() {
		return nil, ErrManagerClosed
	}

	if !m.registry.HasStream(stream) {
		return nil, ErrStreamNotRegistered
	}

	if consumerName == "" {
		return nil, ErrInvalidConsumerConfig
	}

	// Create managed subscription.
	sub := &ManagedSubscription{
		manager:      m,
		stream:       stream,
		consumerName: consumerName,
		handler:      handler,
		factory:      factory,
	}

	// Create subscriber via factory and subscribe.
	subscriber := factory(m.provider)
	if err := subscriber.Subscribe(ctx, handler); err != nil {
		return nil, coreerrs.WrapOperation(err, "subscribe")
	}
	sub.setSubscriber(subscriber)

	// Register for recovery under the server-side consumer name, which the
	// health check looks up; an ephemeral consumer is named by the server.
	if err := m.registry.RegisterSubscription(stream, sub.key, handler, factory); err != nil {
		subscriber.Unsubscribe()
		return nil, err
	}

	// Register resubscribe handler for recovery.
	m.registry.RegisterResubscribeHandler(stream, sub.key, func(recoveryCtx context.Context) error {
		return sub.resubscribe(ctx, recoveryCtx)
	})

	// Track subscription.
	m.subscriptionsMu.Lock()
	m.subscriptions = append(m.subscriptions, sub)
	m.subscriptionsMu.Unlock()

	m.logger.Info("subscription created",
		slog.String("stream", stream),
		slog.String("consumer", consumerName),
		slog.String("subject", handler.Topic()))

	return sub, nil
}

// GetRegistry returns the internal registry.
// Useful for testing and advanced use cases.
func (m *Manager) GetRegistry() *Registry {
	return m.registry
}

// GetSupervisor returns the internal supervisor.
// Useful for testing and advanced use cases.
func (m *Manager) GetSupervisor() *Supervisor {
	return m.supervisor
}

// streamOptions holds options for stream registration.
type streamOptions struct {
	// WithRecoveryStrategy sets the recovery strategy for a stream.
	strategy RecoveryStrategy `opt:"RecoveryStrategy" optgen:"default=RecoveryStrategyAuto"`
}

// StreamOption configures stream registration.
type StreamOption func(*streamOptions)

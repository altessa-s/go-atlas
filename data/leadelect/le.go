// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/data/leadelect/providers"
	"github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	natsio "github.com/nats-io/nats.go"
)

// Leader manages distributed leader election with configurable callbacks.
// It wraps a provider and notifies registered callbacks on leadership changes.
type Leader struct {
	provider providers.Provider
	cfg      Config
	metrics  *leaderMetrics

	lostMu       sync.RWMutex
	onLeaderLost []Callback

	becomeMu        sync.RWMutex
	onBecomesLeader []Callback

	handlerTimeout time.Duration
	handlersWg     sync.WaitGroup
	isRunning      atomic.Bool
}

// New creates a new Leader with the given provider and configuration.
// Default handler timeout is 3 seconds.
//
// Example:
//
//	le := leadelect.New(provider, cfg)
func New(provider providers.Provider, cfg Config, opts ...Option) *Leader {
	options := newOptions(opts...)

	le := &Leader{
		provider:       provider,
		cfg:            cfg,
		metrics:        newLeaderMetrics(options.collector),
		handlerTimeout: options.handlerTimeout,
	}

	return le
}

// NewWithNats creates a Leader configured with a NATS provider.
//
// Example:
//
//	le, err := leadelect.NewWithNats(ctx, conn, cfg)
func NewWithNats(ctx context.Context, conn *natsio.Conn, cfg Config, opts ...Option) (*Leader, error) {
	prov, err := nats.New(ctx, conn)

	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create nats leader elector")
	}

	return New(prov, cfg, opts...), nil
}

// Stop gracefully stops the leader election and waits for running callbacks.
func (le *Leader) Stop(ctx context.Context) error {
	if !le.isRunning.CompareAndSwap(true, false) {
		return nil
	}
	err := le.provider.Stop(ctx)
	le.handlersWg.Wait()

	return err
}

// LeaderId returns the current leader's ID, or empty string if none.
func (le *Leader) LeaderId(ctx context.Context) (string, error) { return le.provider.LeaderId(ctx) }

// IsLeader returns true if this instance is the current leader.
func (le *Leader) IsLeader() bool { return le.provider.IsLeader() }

// NodeId returns this node's unique identifier.
func (le *Leader) NodeId() string { return le.provider.NodeId() }

// IsRunning returns true if the leader election process is active.
func (le *Leader) IsRunning() bool { return le.isRunning.Load() }

// RegisterOnLeaderLost registers a callback for when this instance loses leadership.
// Callbacks are executed concurrently; order is not guaranteed.
func (le *Leader) RegisterOnLeaderLost(handler Callback) {
	le.lostMu.Lock()
	defer le.lostMu.Unlock()
	le.onLeaderLost = append(le.onLeaderLost, handler)
}

// RegisterOnBecomesLeader registers a callback for when this instance becomes leader.
// Callbacks are executed concurrently; order is not guaranteed.
func (le *Leader) RegisterOnBecomesLeader(handler Callback) {
	le.becomeMu.Lock()
	defer le.becomeMu.Unlock()
	le.onBecomesLeader = append(le.onBecomesLeader, handler)
}

// Start initiates the leader election process.
// The context controls the lifetime of the election; cancel it to stop.
//
// Example:
//
//	err := le.Start(ctx)
func (le *Leader) Start(ctx context.Context) error {
	if !le.isRunning.CompareAndSwap(false, true) {
		return nil
	}

	ctx = corecontext.OrBackground(ctx)

	lostCh := make(chan struct{})
	becomeCh := make(chan struct{})
	stopCh := make(chan struct{})

	provCfg := providers.Config{
		Key:      le.cfg.Key,
		TTL:      le.cfg.TTL,
		NodeId:   le.cfg.NodeId,
		LostCh:   lostCh,
		BecameCh: becomeCh,
		StopCh:   stopCh,
	}

	go func() {
		// NOTE: We intentionally do NOT close channels here.
		// The provider sends to these channels from another goroutine.
		// Closing them here would cause "send on closed channel" panic
		// if the provider tries to send after this goroutine exits.
		// Channels will be garbage collected when no longer referenced.
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-stopCh:
				if !ok {
					return
				}
				// Provider signaled stop
				return
			case _, ok := <-becomeCh:
				if !ok {
					return
				}
				le.metrics.transitions.WithLabels(metrics.Labels{"type": "became_leader"}).Inc()
				le.metrics.isLeader.Set(1)
				le.runCallback(ctx, le.onBecomesLeader...)
			case _, ok := <-lostCh:
				if !ok {
					return
				}
				le.metrics.transitions.WithLabels(metrics.Labels{"type": "lost_leader"}).Inc()
				le.metrics.isLeader.Set(0)
				le.runCallback(ctx, le.onLeaderLost...)
			}
		}
	}()

	return le.provider.Start(ctx, provCfg)
}

func (le *Leader) runCallback(ctx context.Context, fn ...Callback) {
	if fn == nil {
		return
	}

	stop := le.metrics.callbackDuration.Start()
	defer stop()

	// Run all callbacks concurrently.
	// Any context cancellation or internal errors are handled by checking the returned error.
	if err := concurrency.Process(ctx, fn, func(ctx context.Context, f Callback) error {
		// Create a new context for each callback with its specific timeout.
		cbCtx, cancel := corecontext.WithMaxTimeout(ctx, le.handlerTimeout)
		defer cancel()

		le.handlersWg.Add(1)
		defer le.handlersWg.Done()

		f(cbCtx, le)

		return nil
	}, concurrency.BatchConfig[Callback]{
		StopOnError: false, // Run all callbacks regardless of errors
	}); err != nil {
		le.metrics.callbackErrors.Inc()
		return
	}
}

// OnBecomesLeaderPrintCallback logs when this node becomes leader.
func OnBecomesLeaderPrintCallback(_ context.Context, _ LeaderElector) {
	slog.Default().Info("node became leader")
}

// OnLeaderLostPrintCallback logs when this node loses leadership.
func OnLeaderLostPrintCallback(ctx context.Context, l LeaderElector) {
	if leaderId, _ := l.LeaderId(ctx); leaderId != "" { // nolint:errcheck
		slog.Default().Info("node lost leadership", slog.String("leader_id", leaderId))
		return
	}
	slog.Default().Info("node lost leadership, no leader ID available")
}

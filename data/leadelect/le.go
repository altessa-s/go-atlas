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
	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/leadelect/providers"
	"github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	natsio "github.com/nats-io/nats.go"
)

// transitionBuffer is the per-channel buffer for leadership transition
// notifications. A leadership change is an edge: if the dispatch goroutine is
// busy running a previous callback at the instant the provider publishes the
// next transition, an unbuffered channel would silently swallow it — losing a
// "leadership lost" edge is how a node keeps acting as leader after it is not.
// The buffer decouples the provider's send from the dispatcher's readiness.
const transitionBuffer = 8

// Leader manages distributed leader election with configurable callbacks.
// It wraps a provider and notifies registered callbacks on leadership changes.
type Leader struct {
	provider providers.Provider
	key      string
	nodeID   string
	ttl      time.Duration
	metrics  *leaderMetrics

	lostMu       sync.RWMutex
	onLeaderLost []Callback

	becomeMu        sync.RWMutex
	onBecomesLeader []Callback

	handlerTimeout time.Duration

	// lifecycleMu serializes Start and Stop so dispatchCancel is never read
	// before the Start that wrote it. A lock-free CAS on isRunning cannot
	// order that write against Stop's read: Stop's CAS only proves Start's CAS
	// already ran, not that Start finished publishing the cancel func.
	lifecycleMu    sync.Mutex
	dispatchCancel context.CancelFunc
	dispatchWg     sync.WaitGroup

	isRunning atomic.Bool
}

// New creates a new Leader for the given election key and node identifier.
// TTL defaults to [DefaultTTL] and can be overridden with [WithTTL].
// Default handler timeout is [DefaultHandlerTimeout].
//
// Example:
//
//	le := leadelect.New(provider, "my-service", "node-1", leadelect.WithTTL(30*time.Second))
func New(provider providers.Provider, key, nodeID string, opts ...Option) *Leader {
	options := newOptions(opts...)

	le := &Leader{
		provider:       provider,
		key:            key,
		nodeID:         nodeID,
		ttl:            options.ttl,
		metrics:        newLeaderMetrics(options.collector),
		handlerTimeout: options.handlerTimeout,
	}

	return le
}

// NewWithNats creates a Leader configured with a NATS provider.
//
// Example:
//
//	le, err := leadelect.NewWithNats(ctx, conn, "my-service", "node-1")
func NewWithNats(ctx context.Context, conn *natsio.Conn, key, nodeID string, opts ...Option) (*Leader, error) {
	prov, err := nats.New(ctx, conn)

	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create nats leader elector")
	}

	return New(prov, key, nodeID, opts...), nil
}

// Stop gracefully stops the leader election and waits for running callbacks.
// Once it returns, no callback is executing and the dispatch goroutine has
// exited. The context bounds the provider's own shutdown (lease resignation).
func (le *Leader) Stop(ctx context.Context) error {
	le.lifecycleMu.Lock()
	defer le.lifecycleMu.Unlock()

	if !le.isRunning.CompareAndSwap(true, false) {
		return nil
	}

	// Stop the provider first, while the dispatch goroutine is still receiving:
	// that is what lets the provider's final "stopped" notification land.
	err := le.provider.Stop(ctx)

	// Then stop the dispatch loop and join it. Callbacks run synchronously
	// inside that goroutine, so joining it — rather than a separate wait group
	// registered after the channel receive — is what makes the "waits for
	// running callbacks" contract actually hold.
	le.dispatchCancel()
	le.dispatchWg.Wait()
	le.dispatchCancel = nil

	le.metrics.isLeader.Set(0)

	return err
}

// LeaderId returns the current leader's ID, or empty string if none.
func (le *Leader) LeaderId(ctx context.Context) (string, error) { return le.provider.LeaderId(ctx) }

// IsLeader returns true if this instance is the current leader.
func (le *Leader) IsLeader() bool { return le.provider.IsLeader() }

// Fence returns the fencing token of the leadership term this node currently
// holds, or 0 when it is not a fresh leader. See [LeaderElector.Fence].
func (le *Leader) Fence() uint64 { return le.provider.Fence() }

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
	le.lifecycleMu.Lock()
	defer le.lifecycleMu.Unlock()

	if le.isRunning.Load() {
		return nil
	}

	ctx = corecontext.OrBackground(ctx)

	lostCh := make(chan struct{}, transitionBuffer)
	becomeCh := make(chan struct{}, transitionBuffer)
	stopCh := make(chan struct{}, 1)

	provCfg := providers.Config{
		Key:      le.key,
		TTL:      le.ttl,
		NodeId:   le.nodeID,
		LostCh:   lostCh,
		BecameCh: becomeCh,
		StopCh:   stopCh,
	}

	// The dispatch loop runs on its own cancelable context so Stop can end it
	// deterministically. Relying on the provider's StopCh alone leaked the
	// goroutine: that notification is best-effort and the caller's context may
	// outlive the election by an arbitrary amount.
	dispatchCtx, cancel := context.WithCancel(ctx)

	le.dispatchWg.Go(func() {
		// Repo rule: every spawned goroutine ships with panics.Handle so
		// a panic inside a user callback (becomeLeader / lostLeader) does
		// not crash the process.
		defer panics.Handle(dispatchCtx)
		le.dispatch(dispatchCtx, becomeCh, lostCh, stopCh)
	})

	if err := le.provider.Start(ctx, provCfg); err != nil {
		cancel()
		le.dispatchWg.Wait()
		return err
	}

	le.dispatchCancel = cancel
	le.isRunning.Store(true)

	return nil
}

// dispatch consumes leadership transitions and runs the registered callbacks.
// Callbacks execute synchronously here, so the goroutine's lifetime is exactly
// the window in which a callback may be running — which is what Stop joins on.
//
// NOTE: the transition channels are intentionally never closed. The provider
// sends to them from another goroutine, so closing them here would risk a
// "send on closed channel" panic. They are garbage collected once unreferenced.
func (le *Leader) dispatch(ctx context.Context, becomeCh, lostCh, stopCh <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-becomeCh:
			le.metrics.transitions.WithLabels(metrics.Labels{"type": "became_leader"}).Inc()
			le.metrics.isLeader.Set(1)
			le.becomeMu.RLock()
			cbs := le.onBecomesLeader
			le.becomeMu.RUnlock()
			le.runCallback(ctx, cbs...)
		case <-lostCh:
			le.metrics.transitions.WithLabels(metrics.Labels{"type": "lost_leader"}).Inc()
			le.metrics.isLeader.Set(0)
			le.lostMu.RLock()
			cbs := le.onLeaderLost
			le.lostMu.RUnlock()
			le.runCallback(ctx, cbs...)
		}
	}
}

func (le *Leader) runCallback(ctx context.Context, fn ...Callback) {
	if len(fn) == 0 {
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

		f(cbCtx, le)

		return nil
	}); err != nil { // No StopOnError: run all callbacks regardless of errors.
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

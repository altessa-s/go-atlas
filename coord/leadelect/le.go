// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas-core/runtime/helpers"
	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corecontext "github.com/altessa-s/go-atlas-core/context"
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

	// startDone is closed when the context passed to the current Start is
	// canceled, which ends the election without a Stop. Guarded by lifecycleMu.
	startDone <-chan struct{}

	isRunning atomic.Bool
	run       atomic.Pointer[electionRun]

	// callbackGoroutines holds the IDs of the goroutines currently running a
	// callback, so Start and Stop can tell a call made from one: such a call
	// must not wait for the callbacks to finish.
	callbackGoroutines sync.Map // int -> struct{}
}

// callbackCtxKey marks the context a callback receives with its Leader, which
// identifies a lifecycle call made from a goroutine the callback started.
type callbackCtxKey struct{}

type electionRun struct {
	stopRequested atomic.Bool
}

// ErrStartFromCallback is returned by Start when it is called from a
// leadership callback. A restart has to wait for the callbacks of the previous
// run, the caller among them; start the Leader from another goroutine instead.
var ErrStartFromCallback = errors.New("leadelect: Start called from a leadership callback")

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
		metrics:        newLeaderMetrics(options.meter),
		handlerTimeout: options.handlerTimeout,
	}

	return le
}

// Stop gracefully stops the leader election and waits for running callbacks.
// Once it returns, no callback is executing and the dispatch goroutine has
// exited. The context bounds the provider's own shutdown (lease resignation).
//
// Stop may be called from a leadership callback, for example to give up after
// a failed leader initialization. It then ends the election without waiting
// for the callbacks — it is one of them — and they finish after it returns.
// Callback delivery begins only after Start releases the lifecycle lock.
// A concurrent lifecycle call honors a callback stop request for the current
// run before completing; the callback never waits for that call.
func (le *Leader) Stop(ctx context.Context) error {
	if le.inCallback(ctx) {
		return le.stopFromCallback(ctx)
	}

	le.lifecycleMu.Lock()
	defer le.unlockLifecycle(ctx)

	if !le.isRunning.Load() {
		// A Stop called from a callback does not join the dispatch goroutine
		// it runs on; this one can.
		le.dispatchWg.Wait()
		return nil
	}

	return le.stop(ctx)
}

// stopFromCallback is Stop for a caller that runs inside the dispatch
// goroutine's callbacks. Joining that goroutine, or waiting for a lifecycle
// call that is itself joining it, would deadlock.
func (le *Leader) stopFromCallback(ctx context.Context) error {
	return le.stopRequestedRun(ctx, le.run.Load())
}

func (le *Leader) stopRequestedRun(ctx context.Context, run *electionRun) error {
	if run == nil {
		return nil
	}
	if !le.lifecycleMu.TryLock() {
		run.stopRequested.Store(true)
		if !le.lifecycleMu.TryLock() {
			return nil
		}
	}
	defer le.unlockLifecycle(ctx)
	if le.run.Load() != run || !le.isRunning.Load() {
		return nil
	}
	return le.stopCallbackRun(ctx)
}

func (le *Leader) unlockLifecycle(ctx context.Context) {
	run := le.run.Load()
	if run != nil && run.stopRequested.Load() {
		_ = le.stopCallbackRun(ctx)
	}
	le.lifecycleMu.Unlock()
	if run != nil && run.stopRequested.Load() {
		_ = le.stopRequestedRun(ctx, run)
	}
}

func (le *Leader) stopCallbackRun(ctx context.Context) error {
	le.run.Store(nil)
	le.isRunning.Store(false)

	err := le.provider.Stop(ctx)

	// The dispatch loop exits once the calling callback returns, and runs the
	// loss callbacks on its way out if the node was leader.
	le.dispatchCancel()
	le.dispatchCancel = nil

	return err
}

// inCallback reports whether the caller runs inside one of this Leader's
// callbacks: on a callback's goroutine, or with a context derived from the one
// a callback received.
func (le *Leader) inCallback(ctx context.Context) bool {
	if ctx != nil && ctx.Value(callbackCtxKey{}) == le {
		return true
	}
	id := helpers.GoroutineID()
	if id == 0 {
		return false
	}
	_, ok := le.callbackGoroutines.Load(id)
	return ok
}

// stop ends the current run. The caller holds lifecycleMu and has checked that
// the Leader was started.
func (le *Leader) stop(ctx context.Context) error {
	le.run.Store(nil)
	le.isRunning.Store(false)

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

// IsRunning returns true if the leader election process is active. The
// provider is the source of truth: an election ended by the context passed to
// Start reports false before Stop is called.
func (le *Leader) IsRunning() bool { return le.isRunning.Load() && le.provider.IsRunning() }

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
//
// Callbacks are delivered only after startup publishes its state and releases
// the lifecycle lock. Start returns nil on successful startup, even if a
// callback immediately stops the election.
// Start returns [ErrStartFromCallback] when called from a leadership callback.
func (le *Leader) Start(ctx context.Context) error {
	if le.inCallback(ctx) {
		return ErrStartFromCallback
	}

	le.lifecycleMu.Lock()
	ready := make(chan struct{})
	defer func() {
		le.unlockLifecycle(ctx)
		close(ready)
	}()

	ctx = corecontext.OrBackground(ctx)

	if le.isRunning.Load() {
		if !le.ended() {
			return nil
		}
		// The previous election was ended by its context rather than by Stop.
		// Returning nil here would leave a node that never campaigns again, so
		// finish that run — which delivers its final loss — and start over.
		if err := le.stop(ctx); err != nil {
			return err
		}
	}

	// A Stop called from a callback leaves its dispatch goroutine to finish on
	// its own. Join it, so two runs never deliver callbacks side by side.
	le.dispatchWg.Wait()

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
	//
	// It is detached from the caller's cancellation: when that context ends
	// the election, the provider still publishes the loss, and the callbacks
	// for it must run rather than be cut off with the context that caused it.
	dispatchCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	le.dispatchWg.Go(func() {
		// Repo rule: every spawned goroutine ships with panics.Handle so
		// a panic inside a user callback (becomeLeader / lostLeader) does
		// not crash the process.
		defer panics.Handle(dispatchCtx)
		select {
		case <-dispatchCtx.Done():
			return
		case <-ready:
		}
		le.dispatch(dispatchCtx, becomeCh, lostCh, stopCh)
	})

	if err := le.provider.Start(ctx, provCfg); err != nil {
		cancel()
		le.dispatchWg.Wait()
		return err
	}

	le.run.Store(&electionRun{})
	le.dispatchCancel = cancel
	le.startDone = ctx.Done()
	le.isRunning.Store(true)

	return nil
}

// ended reports whether the current run's election is over: the context
// passed to Start is canceled or the provider has stopped campaigning. The
// caller holds lifecycleMu.
func (le *Leader) ended() bool {
	select {
	case <-le.startDone:
		return true
	default:
		return !le.provider.IsRunning()
	}
}

// dispatch consumes leadership transitions and runs the registered callbacks.
// Callbacks execute synchronously here, so the goroutine's lifetime is exactly
// the window in which a callback may be running — which is what Stop joins on.
//
// NOTE: the transition channels are intentionally never closed. The provider
// sends to them from another goroutine, so closing them here would risk a
// "send on closed channel" panic. They are garbage collected once unreferenced.
func (le *Leader) dispatch(ctx context.Context, becomeCh, lostCh, stopCh <-chan struct{}) {
	// leader is the state the callbacks were last told about.
	leader := false

	// The election is over once this loop exits, so the node is not leader,
	// whether Stop, the provider's stop or a canceled context ended it. If
	// the callbacks still believe the node leads, tell them: the select below
	// picks at random among ready channels, so the stop can win over the loss
	// that preceded it, and a provider need not publish a loss on Stop at
	// all. Transitions still queued are dropped — they are superseded.
	defer func() {
		if leader {
			le.lostLeader(ctx)
		}
	}()

	// The provider publishes transitions in order, alternating, but on two
	// channels, and a select among ready channels picks at random. When a
	// transition arrives that the current state cannot follow, the one that
	// preceded it is still queued on the other channel: run that one first,
	// so a loss followed by a new term never ends on "lost".
	for {
		// Stop has priority over transitions that are also ready: a new term
		// must not begin on a canceled context, and the loss that ends the
		// current one is delivered on the way out.
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-becomeCh:
			if leader {
				select {
				case <-lostCh:
					le.lostLeader(ctx)
				default:
				}
			}
			leader = true
			le.becameLeader(ctx)
		case <-lostCh:
			if !leader {
				select {
				case <-becomeCh:
					le.becameLeader(ctx)
				default:
				}
			}
			leader = false
			le.lostLeader(ctx)
		}
	}
}

// becameLeader records a new leadership term and runs the leader callbacks.
func (le *Leader) becameLeader(ctx context.Context) {
	le.metrics.transitions.WithLabels(metrics.Labels{labelType: "became_leader"}).Inc()
	le.metrics.isLeader.Set(1)
	le.becomeMu.RLock()
	cbs := le.onBecomesLeader
	le.becomeMu.RUnlock()
	le.runCallback(ctx, cbs...)
}

// lostLeader records a loss of leadership and runs the loss callbacks.
//
// They always run on a context detached from ctx's cancellation: Stop cancels
// the dispatch context while a loss may be queued or already running, and the
// callbacks that stop leader-only work must not be cut short or skipped by it.
// Each callback stays bounded by the handler timeout.
func (le *Leader) lostLeader(ctx context.Context) {
	ctx = context.WithoutCancel(ctx)

	le.metrics.transitions.WithLabels(metrics.Labels{labelType: "lost_leader"}).Inc()
	le.metrics.isLeader.Set(0)
	le.lostMu.RLock()
	cbs := le.onLeaderLost
	le.lostMu.RUnlock()
	le.runCallback(ctx, cbs...)
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

		// Mark the callback, by context and by goroutine, so a Start or Stop
		// it calls on this Leader is recognized instead of deadlocking.
		if id := helpers.GoroutineID(); id != 0 {
			le.callbackGoroutines.Store(id, struct{}{})
			defer le.callbackGoroutines.Delete(id)
		}

		f(context.WithValue(cbCtx, callbackCtxKey{}, le), le)

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

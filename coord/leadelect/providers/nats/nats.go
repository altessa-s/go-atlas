// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/nats/natskey"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrBucketTTLMismatch is returned by New when the bucket already exists with a
// key TTL other than the configured one and [WithMigrateBucketTTL] is not set.
// The bucket is left untouched.
var ErrBucketTTLMismatch = natskvlease.ErrBucketTTLMismatch

// ErrBucketStorageMismatch is returned by New when the bucket already exists
// with another storage type. The bucket is left untouched:
// the server cannot convert a bucket's storage type.
var ErrBucketStorageMismatch = natskvlease.ErrBucketStorageMismatch

// ErrKeyTooLong is returned by Start for an election key whose escaped form
// exceeds the NATS KeyValue key budget (see the package README).
var ErrKeyTooLong = natskey.ErrKeyTooLong

const (
	// minTTL is the shortest election TTL Start accepts. Below a second the
	// renewal interval collapses to a few hundred milliseconds, which is
	// shorter than the round trip a renewal itself needs.
	minTTL = time.Second

	// defaultResignTimeout bounds the lease-resignation call made on the way
	// out of the camping loop when Stop's context carries no deadline of its
	// own. It exists so shutdown cannot hang on an unreachable broker.
	defaultResignTimeout = 2 * time.Second

	// notificationSendTimeout bounds a single notification delivery during the
	// shutdown drain, when the election context is already canceled.
	notificationSendTimeout = time.Second
)

// Provider implements leader election using NATS JetStream KeyValue.
// It is safe for concurrent use.
type Provider struct {
	client  *nats.Conn
	js      jetstream.JetStream
	opts    *options
	metrics *natsLeaderMetrics

	// started pairs Start with Stop. isRunning reports whether the election
	// loop is active: the camping goroutine clears it on its way out, so an
	// election ended by its context reads as stopped before Stop is called.
	started   atomic.Bool
	isRunning atomic.Bool
	isLeader  atomic.Bool

	// lastRenewOK holds the monotonic instant of the last confirmed leadership
	// (a successful acquire or renew). IsLeader treats leadership as expired once
	// the lease elapses since this instant, so a frozen or partitioned camping
	// goroutine — which cannot run its own watchdog — can no longer report stale
	// leadership after the KV key would have expired server-side and been
	// re-acquired elsewhere. Stored as a *time.Time so time.Since uses the
	// monotonic clock and is immune to wall-clock jumps.
	lastRenewOK atomic.Pointer[time.Time]

	// fenceToken holds the KV revision of the lease this node currently holds. The
	// JetStream KV revision is strictly increasing per key, so it doubles as a
	// Kleppmann fencing token: a node that froze while holding revision R can only
	// re-emit R, while whichever node next acquired the key advanced it past R.
	// Stored together with lastRenewOK (token first) so a Fence reader that
	// observes a fresh lease also observes the matching token.
	fenceToken atomic.Uint64

	// lifecycleMu serializes Start and Stop end to end. Each Start installs a
	// fresh notification channel and close guard, which must not be swapped
	// while a concurrent Stop is still tearing down the previous run.
	lifecycleMu sync.Mutex

	stopCtxCancel context.CancelFunc
	wg            sync.WaitGroup

	// electionDone is closed once the current run's election context is
	// canceled, by Stop or by the context passed to Start. Start reads it to
	// tell a run that has ended from one that is still campaigning. Guarded by
	// lifecycleMu.
	electionDone <-chan struct{}

	// stopDeadline carries the shutdown budget from Stop's context to the
	// camping goroutine's resign call, which runs on a context of its own
	// because the election context is already canceled by then. Stored as a
	// bare deadline rather than a context so no context lives in the struct.
	stopDeadline atomic.Pointer[time.Time]

	kv    jetstream.KeyValue
	kvOps *natskvlease.KVOps

	providerConfig atomic.Pointer[providers.Config]

	// notificationCh wakes the notification handler. It carries no payload:
	// the handler delivers the difference between what it last delivered and
	// the state below, so a slow consumer coalesces transitions to the latest
	// state instead of losing the newest one.
	notificationCh chan struct{}
	notificationWg sync.WaitGroup

	notifyMu      sync.Mutex
	notifyLeader  bool   // latest leadership state to deliver
	notifyTerm    uint64 // counts this run's leadership terms; advances with every acquisition
	notifyStopped bool   // the election loop has stopped

	// notificationCloseOnce gates close(p.notificationCh) so a second
	// Stop() (or Stop() racing with a partial-Start failure) cannot
	// panic with "close of closed channel". The notificationHandler
	// goroutine treats a closed channel as a clean shutdown signal so
	// the first close still drives the intended teardown.
	notificationCloseOnce sync.Once
}

var _ providers.Provider = (*Provider)(nil)

// New creates a new NATS JetStream provider.
// Requires NATS server 2.11.0+ with JetStream enabled.
//
// Example:
//
//	provider, err := nats.New(ctx, conn, nats.WithBucket("leaders"))
func New(ctx context.Context, client *nats.Conn, opts ...Option) (*Provider, error) {
	cfg := newOptions(opts...)

	p := &Provider{
		client:         client,
		opts:           cfg,
		metrics:        newNatsLeaderMetrics(cfg.meter),
		notificationCh: make(chan struct{}, 1),
	}

	if !client.IsConnected() {
		return nil, errors.New("NATS client is not connected")
	}

	// Check NATS server version for JetStream support
	if err := natskvlease.ValidateNATSVersion(client); err != nil {
		return nil, err
	}

	var err error
	p.js, err = jetstream.New(client)
	if err != nil {
		if p.opts.logger != nil {
			p.opts.logger.ErrorContext(ctx, "failed to create JetStream context", slog.Any("error", err))
		}
		return nil, err
	}

	if jsErr := natskvlease.ValidateJetStreamEnabled(ctx, p.js, p.opts.logger); jsErr != nil {
		if errors.Is(jsErr, nats.ErrJetStreamNotEnabled) {
			return nil, natskvlease.ErrNATSVersionNotSupported
		}
		return nil, jsErr
	}

	// Use common KV helper for bucket creation
	kvHelper := natskvlease.NewKVHelper(p.js, p.opts.logger)
	p.kv, err = kvHelper.GetOrCreateBucket(ctx, bucketConfig(p.opts))
	if err != nil {
		return nil, err
	}

	p.kvOps = natskvlease.NewKVOps(p.kv, p.opts.logger)

	return p, nil
}

// bucketConfig is the bucket New creates or reconciles.
func bucketConfig(o *options) natskvlease.BucketSpec {
	return natskvlease.BucketSpec{
		Bucket:      o.bucket,
		TTL:         DefaultBucketKeysTTL,
		Storage:     o.storage,
		Compression: true,
		MigrateTTL:  o.migrateBucketTTL,
	}
}

// IsLeader returns true if this instance is the elected leader.
//
// Beyond the cached leadership flag, it enforces a freshness bound: leadership is
// reported only while the time since the last confirmed lease is below the
// effective lease lifetime. This closes the split-brain window where a frozen or
// partitioned camping goroutine would otherwise keep returning a stale true after
// the KV key expired server-side and another node acquired it. The check runs in
// the caller's goroutine, so it holds even when camping itself is starved.
func (p *Provider) IsLeader() bool {
	if !p.isRunning.Load() || !p.isLeader.Load() {
		return false
	}
	cfg := p.providerConfig.Load()
	last := p.lastRenewOK.Load()
	if cfg == nil || last == nil {
		return false
	}
	// The KV key expires after DefaultBucketKeysTTL; never report leadership past
	// the point another node could have re-acquired it, even if the configured
	// election TTL is larger.
	lease := min(cfg.TTL, DefaultBucketKeysTTL)
	return time.Since(*last) < lease
}

// Fence returns the KV revision of the lease this node currently holds, or 0
// when it is not a fresh leader. The revision is strictly increasing per key, so
// it is a valid fencing token: a frozen holder can only re-present its stale
// revision while the node that subsequently acquired the key advanced it.
// Gating on IsLeader means an expired lease (by the freshness bound) reports 0,
// so a zombie leader cannot hand a downstream store a token that still passes.
func (p *Provider) Fence() uint64 {
	if !p.IsLeader() {
		return 0
	}
	return p.fenceToken.Load()
}

// markRenewed records a confirmed lease (successful acquire or renew): its
// fencing token (the KV revision just written) and the monotonic instant, so
// IsLeader can expire stale leadership and Fence can report the term. The token
// is stored before the timestamp so a Fence reader that observes a fresh lease
// via lastRenewOK is guaranteed to also observe the matching token (sync/atomic
// operations are sequentially consistent).
//
// at is the instant captured before the KV call was issued: the server counts
// the key TTL from the write, so stamping the lease after the call returned
// would report leadership past the server-side expiry by the round-trip time.
func (p *Provider) markRenewed(token uint64, at time.Time) {
	p.fenceToken.Store(token)
	p.lastRenewOK.Store(&at)
}

// LeaderId returns the current leader's ID, or empty string if none.
func (p *Provider) LeaderId(ctx context.Context) (string, error) {
	// Load the config once and nil-check it: Start publishes it just after
	// claiming isRunning, so a caller racing that window would otherwise
	// dereference nil.
	cfg := p.providerConfig.Load()
	if !p.isRunning.Load() || cfg == nil {
		return "", providers.ErrProviderStopped
	}

	// IsLeader, not the bare flag: once the lease is past its freshness bound
	// another node may hold the key, and the key itself is then the answer.
	if p.IsLeader() {
		return cfg.NodeId, nil
	}

	entry, err := p.kvOps.Get(ctx, cfg.Key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return "", nil // No leader currently holds the key
		}
		return "", err
	}
	// Key exists, return the value (node ID)
	return string(entry.Value()), nil
}

// NodeId returns this node's unique identifier, or an empty string before
// Start has published a configuration.
func (p *Provider) NodeId() string {
	if cfg := p.providerConfig.Load(); cfg != nil {
		return cfg.NodeId
	}
	return ""
}

// IsRunning returns true if the election process is active.
func (p *Provider) IsRunning() bool {
	return p.isRunning.Load()
}

// Start begins the leader election process with the given configuration.
// Returns immediately; leadership status is updated asynchronously.
func (p *Provider) Start(ctx context.Context, cfg providers.Config) error {
	// Validate before claiming the running state. Claiming first left a
	// rejected Start with isRunning=true and a nil providerConfig: the
	// provider was then permanently unstartable and NodeId panicked on the
	// nil dereference.
	if cfg.Key == "" {
		return errors.New("key cannot be empty")
	}

	// The key stays logical: the KV operations encode it. Checking here only
	// rejects an oversized key before the provider claims to be running.
	if _, err := natskey.Encode(cfg.Key); err != nil {
		return err
	}

	if cfg.NodeId == "" {
		return errors.New("node ID cannot be empty")
	}

	if cfg.TTL < minTTL {
		return fmt.Errorf("TTL must be at least %v, got %v", minTTL, cfg.TTL)
	}

	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()

	if p.started.Load() {
		if !p.ended() {
			return errors.New("provider already running")
		}
		// The previous run was ended by its context rather than by Stop: its
		// state is already final, so join it and start over instead of
		// refusing until the caller remembers to Stop.
		p.join()
	}
	p.started.Store(true)
	p.isRunning.Store(true)

	// A previous Stop closed the notification channel and spent its close
	// guard; reusing them made the restarted run's handler exit at once and
	// its first notification send panic. The previous Stop's deadline must
	// not cap this run's resign either.
	p.notificationCh = make(chan struct{}, 1)
	p.notificationCloseOnce = sync.Once{}
	p.notifyMu.Lock()
	p.notifyLeader, p.notifyTerm, p.notifyStopped = false, 0, false
	p.notifyMu.Unlock()
	p.stopDeadline.Store(nil)

	p.providerConfig.Store(&cfg)

	if p.opts.logger != nil {
		p.opts.logger.DebugContext(ctx, "starting leader election process",
			slog.String("node_id", cfg.NodeId),
			slog.String("key", cfg.Key),
			slog.Duration("ttl", cfg.TTL))
	}

	var stopCtx context.Context
	stopCtx, p.stopCtxCancel = context.WithCancel(ctx)
	p.electionDone = stopCtx.Done()

	// Start notification handler. Each spawned goroutine ships with
	// panics.Handle per the repo rule — without it a panic in a NATS
	// driver callback or in safeChannelSend would crash the process
	// instead of being logged and contained to the affected goroutine.
	p.notificationWg.Go(func() {
		defer panics.Handle(stopCtx)
		p.notificationHandler(stopCtx, &cfg)
	})

	p.wg.Go(func() {
		defer panics.Handle(stopCtx)
		p.camping(stopCtx)
	})

	return nil
}

// Stop stops the election and resigns from leadership if held.
//
// The context bounds the shutdown: its deadline, or [defaultResignTimeout]
// when it has none, caps the lease-resignation call that the camping goroutine
// performs on its way out.
func (p *Provider) Stop(ctx context.Context) error {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()

	if !p.started.CompareAndSwap(true, false) {
		return errors.New("provider already stopped")
	}
	p.isRunning.Store(false)

	// Publish the caller's shutdown budget for camping's resign. Only the
	// deadline is stored, not the context itself — a context must not be
	// parked in a struct, and the deadline is all the resign path needs.
	if deadline, ok := ctx.Deadline(); ok {
		p.stopDeadline.Store(&deadline)
	}

	p.join()

	return nil
}

// ended reports whether the current run's election is over: its context is
// canceled or the camping goroutine has finalized. The caller holds
// lifecycleMu.
func (p *Provider) ended() bool {
	select {
	case <-p.electionDone:
		return true
	default:
		return !p.isRunning.Load()
	}
}

// join ends the current run and waits for its goroutines. The caller holds
// lifecycleMu.
func (p *Provider) join() {
	p.stop()
	p.wg.Wait()

	// The camping goroutine has exited, so leadership is definitively gone.
	// Reflect that in the gauge; otherwise a stopped provider keeps reporting
	// is_leader=1 forever and dashboards show two leaders.
	p.isLeader.Store(false)
	p.metrics.isLeader.Set(0)

	// Close notification channel and wait for handler to finish. The
	// close is gated by sync.Once so a second Stop (or a Stop racing
	// with a failed partial Start) cannot trigger "close of closed
	// channel" — the prior code relied on safeChannelSend's panic
	// recovery as a paper bag for this lifecycle bug instead of
	// closing the door properly.
	p.notificationCloseOnce.Do(func() {
		close(p.notificationCh)
	})
	p.notificationWg.Wait()
}

// notificationHandler delivers leadership notifications in a separate
// goroutine to avoid race conditions with channel operations.
//
// Each wake-up delivers the difference between the state it last delivered
// and the latest one, so transitions that pile up behind a slow consumer
// coalesce: the consumer always ends on the current state and never misses a
// "lost" that is still in effect.
//
// A term change is never coalesced away. When the node lost leadership and
// took it again before the handler ran, the state is "leader" both before and
// after, but another node may have led in between: the consumer that was told
// about the earlier term gets "lost" and then "became" for the new one.
//
// It terminates when notificationCh is closed or once it has delivered the
// "stopped" event, never on ctx cancellation. Selecting on ctx.Done()
// alongside the queue meant that on shutdown — when both are ready — the
// handler could return before draining, so the final "stopped" event was
// routinely lost. Stop closes the channel only after the camping goroutine has
// finished, so no send can race the close.
//
// "Stopped" is the last thing a run publishes, so returning after it loses
// nothing — and it is what releases this goroutine when the election was ended
// by its context and nobody calls Stop to close the channel.
func (p *Provider) notificationHandler(ctx context.Context, cfg *providers.Config) {
	var (
		leader, stopped bool
		term            uint64 // the term of the last delivered "became"
	)
	// deliver reports whether the run has stopped and all of it is delivered.
	deliver := func() bool {
		p.notifyMu.Lock()
		wantLeader, wantTerm, wantStopped := p.notifyLeader, p.notifyTerm, p.notifyStopped
		p.notifyMu.Unlock()

		if leader && (!wantLeader || wantTerm != term) {
			leader = false
			p.safeChannelSend(ctx, cfg.LostCh, "LostCh")
		}
		if wantLeader && !leader {
			leader, term = true, wantTerm
			p.safeChannelSend(ctx, cfg.BecameCh, "BecameCh")
		}
		if wantStopped && !stopped {
			stopped = true
			p.safeChannelSend(ctx, cfg.StopCh, "StopCh")
		}
		return stopped
	}

	for range p.notificationCh {
		if deliver() {
			return
		}
	}
	deliver()
}

// safeChannelSend delivers a notification, recovering the "send on closed
// channel" panic.
//
// The send blocks (bounded by ctx) rather than using select-with-default: a
// leadership transition is an edge, and a non-blocking send dropped it whenever
// the consumer was not parked in its select at that exact instant — which is
// precisely the case while the consumer is running the previous transition's
// callback. Losing a "leadership lost" edge that way leaves the application
// acting as leader after it no longer is.
func (p *Provider) safeChannelSend(ctx context.Context, ch chan<- struct{}, channelName string) {
	if ch == nil {
		return
	}

	// The shutdown drain runs with an already-canceled context. Give it a
	// small independent budget so the final "stopped" event still lands,
	// without reintroducing an unbounded wait on a consumer that has left.
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), notificationSendTimeout)
		defer cancel()
	}

	sent, chClosed := panics.TrySend(ctx, ch, struct{}{})
	if p.opts.logger == nil {
		return
	}
	switch {
	case chClosed:
		p.opts.logger.WarnContext(ctx, "attempting to send on closed channel", slog.String("channel", channelName))
	case !sent:
		p.opts.logger.WarnContext(ctx, "dropped leadership notification: consumer not reading",
			slog.String("channel", channelName))
	}
}

// renewIntervalFor derives the renewal tick from the election TTL and the
// configured ratio.
//
// The lifetime is clamped to the bucket's key TTL first: an election TTL larger
// than that would schedule renewals past the point the server has already aged
// the key out. The ratio clamp — and the reason a non-positive interval must
// never reach time.NewTicker — belongs to every lease, and lives in
// [natskvlease.RenewInterval].
func renewIntervalFor(ttl time.Duration, ratio float64) time.Duration {
	return natskvlease.RenewInterval(min(ttl, DefaultBucketKeysTTL), ratio)
}

// camping is the main loop for leader election.
func (p *Provider) camping(ctx context.Context) {
	config := p.providerConfig.Load()
	renewInterval := renewIntervalFor(config.TTL, p.opts.renewRatio)

	logger := slog.New(slog.DiscardHandler)
	if p.opts.logger != nil {
		logger = p.opts.logger.With(
			slog.String("node_id", config.NodeId),
			slog.String("key", config.Key),
		)
	}

	defer logger.DebugContext(ctx, "camping: leader election loop stopped")

	// Whatever ends the loop — the election context, Stop or a panic — the node
	// stops renewing, so it must stop reporting leadership and running.
	defer func() {
		p.setLeader(false)
		p.isRunning.Store(false)
		p.notifyStop()
	}()

	logger.DebugContext(ctx, "camping: leader election loop started", slog.Duration("ttl", config.TTL),
		slog.Duration("renew_interval", renewInterval))

	// A failed watch must not end the election: the node keeps acquiring and
	// renewing on the ticker and retries the watch on every tick. A nil updates
	// channel never fires, so the loop below is ticker-only until then.
	var (
		watcher jetstream.KeyWatcher
		updates <-chan jetstream.KeyValueEntry
	)
	watch := func() {
		w, err := p.kvOps.Watch(ctx, config.Key)
		if err != nil {
			logger.ErrorContext(ctx, "failed to create key watcher, acquiring on the renew interval only", slog.Any("error", err))
			return
		}
		watcher, updates = w, w.Updates()
	}
	watch()
	defer func() {
		if watcher != nil {
			watcher.Stop() //nolint:errcheck
		}
	}()

	c := &campaign{p: p, ctx: ctx, logger: logger, nodeID: config.NodeId}

	ticker := time.NewTicker(renewInterval) // Attempt to renew more frequently than TTL
	defer ticker.Stop()

	for {
		iterStop := p.metrics.campingIterations.Start()

		select {
		case <-ticker.C:
			if watcher == nil {
				watch()
			}
			if !p.isLeader.Load() {
				c.acquire()
			} else {
				c.renew()
			}
			iterStop()
		case update := <-updates:
			if c.onKeyUpdate(update) {
				iterStop()
			}
		case <-ctx.Done():
			iterStop()
			logger.DebugContext(ctx, "camping: context canceled, resigning from leadership")

			// Give up leadership locally before releasing the key: another node
			// may acquire it the instant it is deleted, and this node must not
			// still report itself leader then.
			p.setLeader(false)

			// The election context is canceled, so resign needs a fresh one —
			// but never an unbounded context.Background(): against an
			// unreachable broker that call blocks for the NATS driver's own
			// timeout (~5s) and blows through the caller's shutdown budget.
			resignCtx, cancelResign := p.resignContext()
			//nolint:contextcheck // deliberately independent of the canceled election context
			if resignErr := p.resign(resignCtx); resignErr != nil {
				logger.ErrorContext(ctx, "camping: failed to resign leadership", slog.Any("error", resignErr))
			}
			cancelResign()

			return
		}
	}
}

// campaign is the state of one [Provider.camping] loop: the election
// context and its logger.
type campaign struct {
	p      *Provider
	ctx    context.Context
	logger *slog.Logger
	nodeID string
}

// acquire tries to take leadership and publishes the outcome.
//
// A failed call says nothing about who holds the key, so it leaves the
// leadership state alone: the lease may still be valid server-side. Leadership
// then ends only when the server answers definitively or the freshness bound
// of [Provider.IsLeader] elapses.
func (c *campaign) acquire() {
	start := time.Now()
	acquired, token, created, err := c.p.acquire(c.ctx)
	if err != nil {
		if coreerrs.IsContextCanceled(err) {
			return
		}
		c.logger.ErrorContext(c.ctx, "camping: failed to acquire leadership", slog.Any("error", err))
		if c.p.isLeader.Load() && !c.p.IsLeader() {
			// The lease outlived its freshness bound without a successful
			// renewal: publish the loss IsLeader already reports.
			c.p.setLeader(false)
		}
		return
	}

	if acquired {
		// Record the confirmed lease before publishing leadership so a
		// concurrent IsLeader sees a fresh timestamp, not a stale/nil one.
		c.p.markRenewed(token, start)
		if created && c.p.isLeader.Load() {
			// The key this node believed it held was gone and had to be
			// created again: the lease was interrupted, and another node may
			// have led in between. setLeader sees no transition, so publish
			// the new term here.
			c.p.notifyLeadership(true)
		}
	}
	c.p.setLeader(acquired)

	if acquired {
		c.logger.DebugContext(c.ctx, "camping: leadership acquired", slog.String("node_id", c.nodeID))
	}
}

// renew extends the lease this node holds, falling back to acquire when the
// renewal fails, and publishes the outcome.
func (c *campaign) renew() {
	start := time.Now()
	renewed, token, err := c.p.renew(c.ctx)
	if err != nil {
		if coreerrs.IsContextCanceled(err) {
			return
		}
		c.logger.ErrorContext(c.ctx, "camping: failed to renew leadership, try acquiring leadership again...", slog.Any("error", err))
		c.acquire()

		return
	}

	if renewed {
		c.p.markRenewed(token, start)
	}
	c.p.setLeader(renewed)
}

// onKeyUpdate reacts to a watcher update of the election key: the end of the
// initial values, or a deletion or purge, triggers an acquisition attempt. It
// reports whether the update counts as a loop iteration (a deletion or purge).
func (c *campaign) onKeyUpdate(update jetstream.KeyValueEntry) bool {
	if update == nil {
		c.acquire()
		return false
	}

	if update.Operation() != jetstream.KeyValueDelete && update.Operation() != jetstream.KeyValuePurge {
		return false
	}

	c.logger.DebugContext(c.ctx, "camping: key update received", slog.String("operation", update.Operation().String()))

	isLeader := c.p.isLeader.Load()
	c.acquire()
	if isLeader && !c.p.isLeader.Load() {
		c.logger.DebugContext(c.ctx, "camping: leadership lost")
	}
	return true
}

// setLeader sets the leader status of this instance and executes any configured callbacks.
// If the leadership status changes, the appropriate callback (BecameCh or LostCh)
// will be executed if configured. This function is resilient to panics that may occur
// if attempting to send on a closed notification channel.
func (p *Provider) setLeader(l bool) {
	wasLeader := p.isLeader.Load()
	p.isLeader.Store(l)

	if l {
		p.metrics.isLeader.Set(1)
	} else {
		p.metrics.isLeader.Set(0)
	}

	if l && !wasLeader { // Became leader
		p.notifyLeadership(true)
	} else if !l && wasLeader {
		// Lost leader. Stop and a canceled election context publish the loss
		// too: the consumer must stop leader-only work however leadership
		// ended, and a "became" it has not been told about yet must not be
		// delivered after the key is released.
		p.notifyLeadership(false)
	}
}

// notifyLeadership records the leadership state the notification handler must
// deliver and wakes it. Every call with leader set starts a new term.
func (p *Provider) notifyLeadership(leader bool) {
	p.notifyMu.Lock()
	p.notifyLeader = leader
	if leader {
		p.notifyTerm++
	}
	p.notifyMu.Unlock()
	p.wakeNotificationHandler()
}

// notifyStop records that the election loop has stopped and wakes the
// notification handler.
func (p *Provider) notifyStop() {
	p.notifyMu.Lock()
	p.notifyStopped = true
	p.notifyMu.Unlock()
	p.wakeNotificationHandler()
}

// wakeNotificationHandler never blocks: a wake-up already pending is enough,
// because the handler reads the latest state when it runs.
func (p *Provider) wakeNotificationHandler() {
	select {
	case p.notificationCh <- struct{}{}:
	default:
	}
}

// acquire attempts to acquire leadership by creating or updating the key.
// Returns true and the KV revision of the held lease (the fencing token) when
// leadership was successfully acquired, or false and 0 otherwise. created
// reports that the key was absent and has been created, as opposed to a key
// this node still held being rewritten: the lease did not carry over, so the
// acquisition starts a new term.
func (p *Provider) acquire(ctx context.Context) (acquired bool, token uint64, created bool, err error) {
	config := p.providerConfig.Load()

	// Create succeeds only while the key is absent, so winning it outright is
	// the whole acquisition — and its revision is already the fencing token.
	//
	// This used to re-read and rewrite the key afterwards, which cost two extra
	// round trips and a revision per election, and could report the acquisition
	// as failed when only that redundant rewrite failed — leaving the node
	// holding the key while believing it had lost.
	rev, err := p.kvOps.Create(ctx, config.Key, []byte(config.NodeId))
	if err == nil {
		p.recordLease("acquire", nil)

		return true, rev, true, nil
	}
	if !errors.Is(err, jetstream.ErrKeyExists) {
		p.recordLease("acquire", err)

		return false, 0, false, err
	}

	// The key is taken. It is either still ours from a previous term — in which
	// case renewing it keeps the term alive — or another node's, and we lose.
	entry, err := p.kvOps.Get(ctx, config.Key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			// It expired between the two calls. The key watcher fires on that
			// expiry and drives another attempt, so there is nothing to do here
			// but report the miss.
			p.recordLease("acquire", err)

			return false, 0, false, nil
		}
		p.recordLease("acquire", err)

		return false, 0, false, err
	}

	if string(entry.Value()) != config.NodeId {
		p.recordLease("acquire", errKeyHeldByOther)

		return false, 0, false, nil
	}

	rev, err = p.kvOps.Update(ctx, config.Key, []byte(config.NodeId), entry.Revision())
	p.recordLease("acquire", err)

	return err == nil, rev, false, err
}

// errKeyHeldByOther marks a lost election for metric classification only; it is
// never returned to a caller, because losing is an outcome rather than a fault.
var errKeyHeldByOther = errors.New("election key held by another node")

// recordLease counts one lease operation, classifying it by whether it failed.
func (p *Provider) recordLease(op string, err error) {
	result := "success"
	if err != nil {
		result = "failure"
	}
	p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": op, "result": result}).Inc()
}

// resign gives up leadership by deleting the key if this node is the current leader.
// Returns an error if there was a problem communicating with NATS, but will return
// nil if the key is already deleted or if this node is not the leader.
func (p *Provider) resign(ctx context.Context) error {
	config := p.providerConfig.Load()

	entry, err := p.kvOps.Get(ctx, config.Key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, nats.ErrBucketNotFound) ||
			errors.Is(err, nats.ErrNoStreamResponse) {
			p.recordLease("resign", nil)
			return nil
		}
		p.recordLease("resign", err)
		return err
	}

	// Only attempt delete if Get succeeds and we are the leader
	if string(entry.Value()) == config.NodeId {
		if err := p.kvOps.DeleteWithRevision(ctx, config.Key, entry.Revision()); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				p.recordLease("resign", nil)
				return nil
			}
			p.recordLease("resign", err)
			return err
		}
	}

	p.recordLease("resign", nil)
	return nil
}

// renew attempts to renew the lease by updating the key with the same value.
// Returns true and the new KV revision (the refreshed fencing token) on success,
// or false and 0 otherwise.
func (p *Provider) renew(ctx context.Context) (bool, uint64, error) {
	config := p.providerConfig.Load()

	entry, err := p.kvOps.Get(ctx, config.Key)
	if err != nil {
		p.recordLease("renew", err)
		return false, 0, err
	}

	// Check if we are still the owner before renewing
	if entry == nil || string(entry.Value()) != config.NodeId {
		p.recordLease("renew", errKeyHeldByOther)

		return false, 0, nil
	}

	// Update the key with the same value, which resets the TTL.
	rev, err := p.kvOps.Update(ctx, config.Key, []byte(config.NodeId), entry.Revision())
	p.recordLease("renew", err)

	return err == nil, rev, err
}

func (p *Provider) stop() {
	p.stopCtxCancel()
}

// resignContext builds the bounded context used to release the lease while
// shutting down. The deadline is the earlier of the budget Stop published from
// its own context and [defaultResignTimeout], so a caller that asks for a
// tighter shutdown gets one and a caller that asks for nothing still gets a
// bound.
func (p *Provider) resignContext() (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(defaultResignTimeout)
	if stopBy := p.stopDeadline.Load(); stopBy != nil && stopBy.Before(deadline) {
		deadline = *stopBy
	}

	return context.WithDeadline(context.Background(), deadline)
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
	"github.com/altessa-s/go-atlas/data/leadelect/providers"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	lerrs "github.com/altessa-s/go-atlas/data/leadelect/errs"
)

type notificationEventType int

const (
	eventBecameLeader notificationEventType = iota
	eventLostLeader
	eventStopped
)

type notificationEvent struct {
	eventType notificationEventType
	becameCh  chan<- struct{}
	lostCh    chan<- struct{}
	stopCh    chan<- struct{}
}

// Provider implements leader election using NATS JetStream KeyValue.
// It is safe for concurrent use.
type Provider struct {
	client  *nats.Conn
	js      jetstream.JetStream
	opts    *options
	metrics *natsLeaderMetrics

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

	stopCtxCancel context.CancelFunc
	wg            sync.WaitGroup

	kv    jetstream.KeyValue
	kvOps *natskvlease.KVOps

	providerConfig atomic.Pointer[providers.Config]
	notificationCh chan notificationEvent
	notificationWg sync.WaitGroup

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
		metrics:        newNatsLeaderMetrics(cfg.collector),
		notificationCh: make(chan notificationEvent, 10), // Buffered to avoid blocking
	}

	if !client.IsConnected() {
		return nil, errors.New("NATS client is not connected")
	}

	// Check NATS server version for JetStream support
	if err := natskvlease.ValidateNatsVersion(client); err != nil {
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
			return nil, natskvlease.ErrNatsVersionNotSupported
		}
		return nil, jsErr
	}

	// Use common KV helper for bucket creation
	kvHelper := natskvlease.NewKVHelper(p.js, p.opts.logger)
	p.kv, err = kvHelper.GetOrCreateBucket(ctx, natskvlease.BucketConfig{
		Bucket:      p.opts.bucket,
		TTL:         DefaultBucketKeysTTL,
		Storage:     p.opts.storage,
		Compression: true,
	})
	if err != nil {
		return nil, err
	}

	p.kvOps = natskvlease.NewKVOps(p.kv, p.opts.logger)

	return p, nil
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
func (p *Provider) markRenewed(token uint64) {
	now := time.Now()
	p.fenceToken.Store(token)
	p.lastRenewOK.Store(&now)
}

// LeaderId returns the current leader's ID, or empty string if none.
func (p *Provider) LeaderId(ctx context.Context) (string, error) {
	if !p.isRunning.Load() {
		return "", lerrs.ErrProviderStopped
	}

	if p.isLeader.Load() {
		return p.providerConfig.Load().NodeId, nil
	}

	entry, err := p.kvOps.Get(ctx, p.providerConfig.Load().Key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return "", nil // No leader currently holds the key
		}
		return "", err
	}
	// Key exists, return the value (node ID)
	return string(entry.Value()), nil
}

// NodeId returns this node's unique identifier.
func (p *Provider) NodeId() string {
	return p.providerConfig.Load().NodeId
}

// IsRunning returns true if the election process is active.
func (p *Provider) IsRunning() bool {
	return p.isRunning.Load()
}

// Start begins the leader election process with the given configuration.
// Returns immediately; leadership status is updated asynchronously.
func (p *Provider) Start(ctx context.Context, cfg providers.Config) error {
	if !p.isRunning.CompareAndSwap(false, true) {
		return errors.New("provider already running")
	}

	if cfg.Key == "" {
		return errors.New("key cannot be empty")
	}

	if cfg.NodeId == "" {
		return errors.New("node ID cannot be empty")
	}

	if cfg.TTL < time.Second {
		return errors.New("TTL must be greater than zero")
	}

	p.providerConfig.Store(&cfg)

	if p.opts.logger != nil {
		p.opts.logger.DebugContext(ctx, "starting leader election process",
			slog.String("node_id", cfg.NodeId),
			slog.String("key", cfg.Key),
			slog.Duration("ttl", cfg.TTL))
	}

	var stopCtx context.Context
	stopCtx, p.stopCtxCancel = context.WithCancel(ctx)

	// Start notification handler. Each spawned goroutine ships with
	// panics.Handle per the repo rule — without it a panic in a NATS
	// driver callback or in safeChannelSend would crash the process
	// instead of being logged and contained to the affected goroutine.
	p.notificationWg.Go(func() {
		defer panics.Handle(stopCtx)
		p.notificationHandler(stopCtx)
	})

	p.wg.Go(func() {
		defer panics.Handle(stopCtx)
		p.camping(stopCtx)
	})

	return nil
}

// Stop stops the election and resigns from leadership if held.
func (p *Provider) Stop(_ context.Context) error {
	if !p.isRunning.CompareAndSwap(true, false) {
		return errors.New("provider already stopped")
	}

	p.stop()
	p.wg.Wait()

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

	return nil
}

// notificationHandler processes notification events in a separate goroutine
// to avoid race conditions with channel operations
func (p *Provider) notificationHandler(ctx context.Context) {
	for {
		select {
		case event, ok := <-p.notificationCh:
			if !ok {
				return // Channel closed, shutdown
			}

			// Process the event safely
			switch event.eventType {
			case eventBecameLeader:
				p.safeChannelSend(ctx, event.becameCh, "BecameCh")
			case eventLostLeader:
				p.safeChannelSend(ctx, event.lostCh, "LostCh")
			case eventStopped:
				p.safeChannelSend(ctx, event.stopCh, "StopCh")
			}

		case <-ctx.Done():
			return
		}
	}
}

// safeChannelSend safely sends to a channel with panic recovery
func (p *Provider) safeChannelSend(ctx context.Context, ch chan<- struct{}, channelName string) {
	if ch == nil {
		return
	}

	// Non-blocking send
	if _, chClosed := panics.TrySendNonBlocking(ch, struct{}{}); chClosed {
		if p.opts.logger != nil {
			p.opts.logger.WarnContext(ctx, "attempting to send on closed channel", slog.String("channel", channelName))
		}
	}
}

// camping is the main loop for leader election.
func (p *Provider) camping(ctx context.Context) {
	config := p.providerConfig.Load()
	renewInterval := min(config.TTL, DefaultBucketKeysTTL)
	renewInterval = time.Duration(float64(renewInterval) * p.opts.renewRatio)

	// Capture channels at the start to avoid race conditions with channel closure
	becameCh := config.BecameCh
	lostCh := config.LostCh
	stopCh := config.StopCh

	logger := slog.New(slog.DiscardHandler)
	if p.opts.logger != nil {
		logger = p.opts.logger.With(
			slog.String("node_id", config.NodeId),
			slog.String("key", config.Key),
		)
	}

	defer logger.DebugContext(ctx, "camping: leader election loop stopped")

	logger.DebugContext(ctx, "camping: leader election loop started", slog.Duration("ttl", config.TTL),
		slog.Duration("renew_interval", renewInterval))

	watcher, err := p.kvOps.Watch(ctx, config.Key)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create key watcher", slog.Any("error", err))
		return
	}
	defer watcher.Stop() //nolint:errcheck

	acquire := func() {
		acquired, token, err := p.acquire(ctx)
		if acquired {
			// Record the confirmed lease before publishing leadership so a
			// concurrent IsLeader sees a fresh timestamp, not a stale/nil one.
			p.markRenewed(token)
		}
		p.setLeader(ctx, acquired, becameCh, lostCh)

		if err != nil {
			if coreerrs.IsContextCanceled(err) {
				return
			}
			logger.ErrorContext(ctx, "camping: failed to acquire leadership", slog.Any("error", err))
		} else if acquired {
			logger.DebugContext(ctx, "camping: leadership acquired", slog.String("node_id", config.NodeId))
		}
	}

	renew := func() {
		renewed, token, err := p.renew(ctx)
		if err != nil {
			if coreerrs.IsContextCanceled(err) {
				return
			}
			logger.ErrorContext(ctx, "camping: failed to renew leadership, try acquiring leadership again...", slog.Any("error", err))
			acquire()

			return
		}

		if renewed {
			p.markRenewed(token)
		}
		p.setLeader(ctx, renewed, becameCh, lostCh)
	}

	ticker := time.NewTicker(renewInterval) // Attempt to renew more frequently than TTL
	defer ticker.Stop()

	for {
		iterStop := p.metrics.campingIterations.Start()

		select {
		case <-ticker.C:
			if !p.isLeader.Load() {
				acquire()
			} else {
				renew()
			}
			iterStop()
		case update := <-watcher.Updates():
			if update == nil {
				acquire()
				continue
			}

			if update.Operation() != jetstream.KeyValueDelete && update.Operation() != jetstream.KeyValuePurge {
				continue
			}

			logger.DebugContext(ctx, "camping: key update received", slog.String("operation", update.Operation().String()))

			isLeader := p.isLeader.Load()
			acquire()
			if isLeader && !p.isLeader.Load() {
				logger.DebugContext(ctx, "camping: leadership lost")
			}
			iterStop()

		case <-ctx.Done():
			iterStop()
			logger.DebugContext(ctx, "camping: context canceled, resigning from leadership")

			_ = p.resign(context.Background()) //nolint:errcheck,contextcheck

			p.queueNotification(ctx, eventStopped, nil, nil, stopCh)
			return
		}
	}
}

// setLeader sets the leader status of this instance and executes any configured callbacks.
// If the leadership status changes, the appropriate callback (BecameCh or LostCh)
// will be executed if configured. This function is resilient to panics that may occur
// if attempting to send on a closed notification channel.
func (p *Provider) setLeader(ctx context.Context, l bool, becameCh, lostCh chan<- struct{}) {
	wasLeader := p.isLeader.Load()
	p.isLeader.Store(l)

	if l {
		p.metrics.isLeader.Set(1)
	} else {
		p.metrics.isLeader.Set(0)
	}

	if l && !wasLeader { // Became leader
		p.queueNotification(ctx, eventBecameLeader, becameCh, lostCh, nil)
	} else if !l && wasLeader && p.IsRunning() { // Lost leader (and provider is still considered running)
		p.queueNotification(ctx, eventLostLeader, becameCh, lostCh, nil)
	}
}

// queueNotification queues a notification event to be processed by the notification handler
func (p *Provider) queueNotification(ctx context.Context, eventType notificationEventType, becameCh, lostCh, stopCh chan<- struct{}) {
	event := notificationEvent{
		eventType: eventType,
		becameCh:  becameCh,
		lostCh:    lostCh,
		stopCh:    stopCh,
	}

	select {
	case p.notificationCh <- event:
		// Event queued successfully
	default:
		// Channel is full, log a warning but don't block
		if p.opts.logger != nil {
			p.opts.logger.WarnContext(ctx, "notification channel is full, dropping event")
		}
	}
}

// acquire attempts to acquire leadership by creating or updating the key.
// Returns true and the KV revision of the held lease (the fencing token) when
// leadership was successfully acquired, or false and 0 otherwise.
func (p *Provider) acquire(ctx context.Context) (bool, uint64, error) {
	config := p.providerConfig.Load()

	// Attempt to create the key - this works only if the key doesn't exist.
	_, err := p.kvOps.Create(ctx, config.Key, []byte(config.NodeId))
	if err != nil && !errors.Is(err, jetstream.ErrKeyExists) {
		p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "acquire", "result": "failure"}).Inc()
		return false, 0, err
	}

	entry, getErr := p.kvOps.Get(ctx, config.Key)
	if getErr == nil && string(entry.Value()) == config.NodeId {
		// We are the current leader, so we can update the key to renew the lease.
		rev, updateErr := p.kvOps.Update(ctx, config.Key, []byte(config.NodeId), entry.Revision())
		if updateErr == nil {
			p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
		} else {
			p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "acquire", "result": "failure"}).Inc()
		}
		return updateErr == nil, rev, updateErr
	} else if errors.Is(getErr, jetstream.ErrKeyNotFound) {
		rev, createErr := p.kvOps.Create(ctx, config.Key, []byte(config.NodeId))
		if createErr == nil {
			p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
		} else {
			p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "acquire", "result": "failure"}).Inc()
		}
		return createErr == nil, rev, createErr
	}

	p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "acquire", "result": "failure"}).Inc()
	return false, 0, getErr
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
			p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "resign", "result": "success"}).Inc()
			return nil
		}
		p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "resign", "result": "failure"}).Inc()
		return err
	}

	// Only attempt delete if Get succeeds and we are the leader
	if string(entry.Value()) == config.NodeId {
		if err := p.kvOps.DeleteWithRevision(ctx, config.Key, entry.Revision()); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "resign", "result": "success"}).Inc()
				return nil
			}
			p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "resign", "result": "failure"}).Inc()
			return err
		}
	}

	p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "resign", "result": "success"}).Inc()
	return nil
}

// renew attempts to renew the lease by updating the key with the same value.
// Returns true and the new KV revision (the refreshed fencing token) on success,
// or false and 0 otherwise.
func (p *Provider) renew(ctx context.Context) (bool, uint64, error) {
	config := p.providerConfig.Load()

	entry, err := p.kvOps.Get(ctx, config.Key)
	if err != nil {
		p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
		return false, 0, err
	}

	// Check if we are still the owner before renewing
	if entry == nil || string(entry.Value()) != config.NodeId {
		p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
		return false, 0, nil
	}

	// Update the key with the same value, which resets the TTL.
	rev, err := p.kvOps.Update(ctx, config.Key, []byte(config.NodeId), entry.Revision())
	if err == nil {
		p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "renew", "result": "success"}).Inc()
	} else {
		p.metrics.leaseOperations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
	}
	return err == nil, rev, err
}

func (p *Provider) stop() {
	p.stopCtxCancel()
}

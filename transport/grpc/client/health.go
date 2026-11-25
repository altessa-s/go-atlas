// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/observability/health"

	"google.golang.org/grpc/connectivity"

	pool "github.com/altessa-s/go-atlas/transport/grpc/client/pool"
)

// clientHealth implements the optional [observability/health] integration
// for [Client]. Behavior depends on the client's mode:
//
//   - Single-connection mode: a watcher goroutine calls
//     [grpc.ClientConn.WaitForStateChange] on the client-owned conn and
//     pushes state-derived statuses to the coordinator.
//   - Pool mode: the helper subscribes to [pool.ConnectionPool.SubscribeTarget]
//     for the client's target address and forwards each push.
//
// Without a coordinator the helper is nil and its methods are no-ops.
type clientHealth struct {
	client      *Client
	serviceName string
	mapper      pool.StateMapper // never nil

	stopWatching func()

	// A pool may publish during subscription, before registration succeeds.
	registered atomic.Bool
	notify     func(health.ServingStatus)
	unregister func()

	// detachOnce serializes teardown so concurrent Close calls neither race
	// on the fields above nor return before the watcher has exited.
	detachOnce sync.Once
}

// newClientHealth wires options into a *clientHealth. Returns nil when the
// integration is disabled so the request hot path can do a single nil
// check at the call site.
func newClientHealth(c *Client) *clientHealth {
	if c.options.scopedHealthReporter == nil && c.options.healthCoordinator == nil {
		return nil
	}
	return &clientHealth{
		client:      c,
		serviceName: c.options.healthServiceName,
		mapper:      c.options.healthStateMapper,
	}
}

// attach registers the client checker and starts the appropriate watcher.
func (h *clientHealth) attach(ctx context.Context) error {
	if h == nil {
		return nil
	}
	return h.client.backend.attachHealth(ctx, h)
}

func (b *pooledBackend) attachHealth(ctx context.Context, h *clientHealth) error {
	//nolint:contextcheck // The subscription lives until the client closes.
	unsub, err := b.pool.SubscribeTarget(b.target, h.onPoolStateChange)
	if err != nil {
		return err
	}
	h.stopWatching = unsub
	h.register()
	h.notify(h.CheckHealth(ctx))
	return nil
}

//nolint:contextcheck // The client owns its watcher until Close.
func (b *singleBackend) attachHealth(_ context.Context, h *clientHealth) error {
	h.register()
	watcherCtx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		state := b.GetState()
		h.notify(h.mapper(state))
		for b.WaitForStateChange(watcherCtx, state) {
			state = b.GetState()
			h.notify(h.mapper(state))
		}
	})
	h.stopWatching = func() { cancel(); wg.Wait() }
	return nil
}

func (h *clientHealth) register() {
	if scoped := h.client.options.scopedHealthReporter; scoped != nil {
		h.notify, h.unregister = scoped.RegisterScopedService(h.serviceName, h)
	} else {
		h.notify, h.unregister = health.RegisterOwnedService(h.client.options.healthCoordinator, h.serviceName, h)
	}
	h.registered.Store(true)
}

// detach stops the watcher / unsubscribes. Idempotent and safe for
// concurrent use: every caller returns only after teardown has completed.
func (h *clientHealth) detach() {
	if h == nil {
		return
	}
	h.detachOnce.Do(func() {
		h.registered.Store(false)
		if h.stopWatching != nil {
			h.stopWatching()
			h.stopWatching = nil
		}
		if h.unregister != nil {
			h.unregister()
		}
	})
}

// onPoolStateChange is the subscription callback used in pool mode. The
// pool's tracker invokes it synchronously on every per-conn state change.
func (h *clientHealth) onPoolStateChange(state connectivity.State) {
	if h.registered.Load() {
		h.notify(h.mapper(state))
	}
}

// CheckHealth implements [health.Checker]. The coordinator's pull path uses
// it directly; push notifications above call the same mapping.
func (h *clientHealth) CheckHealth(_ context.Context) health.ServingStatus {
	if h == nil {
		return health.StatusServing
	}
	if h.client.backend == nil {
		return health.StatusUnknown
	}
	return h.mapper(h.client.backend.GetState())
}

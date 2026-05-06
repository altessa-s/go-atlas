// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"sync"

	"github.com/altessa-s/go-atlas/observability/health"

	"google.golang.org/grpc/connectivity"
)

// clientHealth implements the optional [observability/health] integration
// for [Client]. Behaviour depends on the client's mode:
//
//   - Single-connection mode: a watcher goroutine calls
//     [grpc.ClientConn.WaitForStateChange] on the client-owned conn and
//     pushes state-derived statuses to the coordinator.
//   - Pool mode: the helper subscribes to [pool.ConnectionPool.SubscribeTarget]
//     for the client's target address and forwards each push.
//
// The helper is allocated even when no coordinator is configured, in which
// case all methods are cheap noops.
type clientHealth struct {
	client      *Client
	coordinator *health.Coordinator // may be nil
	serviceName string
	mapper      StateMapper // never nil

	// single-mode resources
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// pool-mode resources
	poolUnsub func()
}

// newClientHealth wires options into a *clientHealth. Returns nil when the
// integration is disabled so the request hot path can do a single nil
// check at the call site.
func newClientHealth(c *Client) *clientHealth {
	if c.options.healthCoordinator == nil {
		return nil
	}
	return &clientHealth{
		client:      c,
		coordinator: c.options.healthCoordinator,
		serviceName: c.options.healthServiceName,
		mapper:      c.options.healthStateMapper,
	}
}

// attach registers the client checker and starts the appropriate watcher.
// Must be called from [Client.connect] after c.conn is set (single mode) or
// after pool mode is confirmed (pool mode).
func (h *clientHealth) attach(ctx context.Context) error {
	if h == nil {
		return nil
	}
	h.coordinator.RegisterService(h.serviceName, h)

	if h.client.options.pool != nil {
		unsub, err := h.client.options.pool.SubscribeTarget(h.client.address, h.onPoolStateChange)
		if err != nil {
			return err
		}
		h.poolUnsub = unsub
		// Push the current snapshot immediately so subscribers see a
		// status without waiting for the first transition.
		h.coordinator.NotifyStatusChange(h.serviceName, h.CheckHealth(ctx))
		return nil
	}

	// Single mode: own watcher goroutine.
	watcherCtx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.wg.Add(1)
	go h.watchSingle(watcherCtx)
	return nil
}

// detach stops the watcher / unsubscribes. Idempotent.
func (h *clientHealth) detach() {
	if h == nil {
		return
	}
	if h.poolUnsub != nil {
		h.poolUnsub()
		h.poolUnsub = nil
	}
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
		h.wg.Wait()
	}
}

func (h *clientHealth) watchSingle(ctx context.Context) {
	defer h.wg.Done()
	if h.client.conn == nil {
		return
	}
	state := h.client.conn.GetState()
	h.coordinator.NotifyStatusChange(h.serviceName, h.mapper(state))
	for h.client.conn.WaitForStateChange(ctx, state) {
		state = h.client.conn.GetState()
		h.coordinator.NotifyStatusChange(h.serviceName, h.mapper(state))
	}
}

// onPoolStateChange is the subscription callback used in pool mode. The
// pool's tracker invokes it synchronously on every per-conn state change.
func (h *clientHealth) onPoolStateChange(state connectivity.State) {
	h.coordinator.NotifyStatusChange(h.serviceName, h.mapper(state))
}

// CheckHealth implements [health.Checker]. The coordinator's pull path uses
// it directly; push notifications above call the same mapping.
func (h *clientHealth) CheckHealth(_ context.Context) health.ServingStatus {
	if h == nil {
		return health.StatusServing
	}
	if h.client.options.pool != nil {
		return h.mapper(h.client.options.pool.StateForTarget(h.client.address))
	}
	if h.client.conn == nil {
		return health.StatusUnknown
	}
	return h.mapper(h.client.conn.GetState())
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pool

import (
	"context"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/observability/health"

	"google.golang.org/grpc/connectivity"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// DefaultPoolHealthServiceName is the service name used when the pool
// registers its aggregate checker with [observability/health.Coordinator].
// It mirrors [DefaultMetricsSubsystem] for visual consistency in dashboards.
const DefaultPoolHealthServiceName = "grpc_pool"

// StateMapper maps a [connectivity.State] into a [health.ServingStatus].
// Configure a custom one via [WithHealthStateMapper] on either the pool or
// the gRPC client.
type StateMapper = func(connectivity.State) health.ServingStatus

// DefaultStateMapper is the default mapping. [TransientFailure] reports
// Degraded (gRPC reconnects automatically) and [Shutdown] reports
// NotServing. [Ready], [Idle], and [Connecting] all map to Serving so a
// freshly-dialed pool is not flapping during the initial handshake.
func DefaultStateMapper(s connectivity.State) health.ServingStatus {
	switch s {
	case connectivity.Ready, connectivity.Idle, connectivity.Connecting:
		return health.StatusServing
	case connectivity.TransientFailure:
		return health.StatusDegraded
	case connectivity.Shutdown:
		return health.StatusNotServing
	default:
		return health.StatusUnknown
	}
}

// poolHealth is the optional pool-level health helper. It owns the wiring
// between the [stateTracker] and the user-supplied
// [observability/health.Coordinator]. When no coordinator is configured the
// helper is still used to back [ConnectionPool.CheckHealth] and
// [ConnectionPool.StateForTarget] using the default mapper.
type poolHealth struct {
	pool        *ConnectionPool
	coordinator *health.Coordinator // may be nil
	serviceName string
	mapper      StateMapper // never nil
	perTarget   bool

	perTargetMu       sync.Mutex
	registeredTargets map[string]int // target -> live conn count for lazy registration
}

func newPoolHealth(p *ConnectionPool) *poolHealth {
	mapper := p.opts.healthStateMapper
	if mapper == nil {
		mapper = DefaultStateMapper
	}
	return &poolHealth{
		pool:              p,
		coordinator:       p.opts.healthCoordinator,
		serviceName:       p.opts.healthServiceName,
		mapper:            mapper,
		perTarget:         p.opts.healthPerTarget,
		registeredTargets: make(map[string]int),
	}
}

// register hooks the helper into the tracker and, when a coordinator is
// configured, registers the aggregate service. Per-target services are
// registered lazily as new targets first appear via [onConnAttached].
func (h *poolHealth) register() {
	h.pool.tracker.onTargetStateChange = h.onTargetStateChange
	if h.coordinator == nil {
		return
	}
	h.coordinator.RegisterService(h.serviceName, h)
	h.pool.tracker.enable()
}

// onTargetStateChange is the synchronous hook the tracker calls on every
// per-conn state change. It pushes the recomputed aggregate and, when
// per-target tracking is on, the per-target status.
func (h *poolHealth) onTargetStateChange(target string, _ connectivity.State) {
	if h.coordinator == nil {
		return
	}
	h.coordinator.NotifyStatusChange(h.serviceName, h.aggregateStatus())
	if h.perTarget && h.targetRegistered(target) {
		h.coordinator.NotifyStatusChange(perTargetServiceName(h.serviceName, target), h.targetStatus(target))
	}
}

// onConnAttached is called from [targetPool.createConnection] after the
// tracker has been told about pc. It registers a per-target service the
// first time we see a target.
func (h *poolHealth) onConnAttached(target string) {
	if h.coordinator == nil || !h.perTarget {
		return
	}
	h.perTargetMu.Lock()
	count := h.registeredTargets[target] + 1
	h.registeredTargets[target] = count
	h.perTargetMu.Unlock()
	if count == 1 {
		h.coordinator.RegisterService(perTargetServiceName(h.serviceName, target), &perTargetChecker{ph: h, target: target})
	}
}

// onConnDetached is the mirror of onConnAttached. The per-target service is
// unregistered when its last conn disappears so the registry stays bounded.
func (h *poolHealth) onConnDetached(target string) {
	if h.coordinator == nil || !h.perTarget {
		return
	}
	h.perTargetMu.Lock()
	count := h.registeredTargets[target] - 1
	if count <= 0 {
		delete(h.registeredTargets, target)
	} else {
		h.registeredTargets[target] = count
	}
	h.perTargetMu.Unlock()
	if count <= 0 {
		h.coordinator.UnregisterService(perTargetServiceName(h.serviceName, target))
	}
}

func (h *poolHealth) targetRegistered(target string) bool {
	h.perTargetMu.Lock()
	defer h.perTargetMu.Unlock()
	_, ok := h.registeredTargets[target]
	return ok
}

// CheckHealth implements [health.Checker] for the aggregate pool service.
func (h *poolHealth) CheckHealth(_ context.Context) health.ServingStatus {
	return h.aggregateStatus()
}

// aggregateStatus returns the worst per-target status across the pool. With
// no tracked targets it returns Serving — the pool is empty but functional.
func (h *poolHealth) aggregateStatus() health.ServingStatus {
	targets := h.pool.tracker.allTargets()
	if len(targets) == 0 {
		return health.StatusServing
	}
	worst := health.StatusServing
	for _, t := range targets {
		s := h.targetStatus(t)
		if statusRank(s) < statusRank(worst) {
			worst = s
		}
	}
	return worst
}

// targetStatus returns the best (most-ready) status for a target.
func (h *poolHealth) targetStatus(target string) health.ServingStatus {
	return h.mapper(h.pool.tracker.stateForTarget(target))
}

// statusRank ranks ServingStatus from worst (NotServing) to best (Serving)
// so [aggregateStatus] can pick the worst.
func statusRank(s health.ServingStatus) int {
	//nolint:mnd // ordinal ranks for health.ServingStatus
	switch s {
	case health.StatusServing:
		return 3
	case health.StatusDegraded:
		return 2
	case health.StatusNotServing:
		return 1
	default:
		return 0
	}
}

// perTargetChecker adapts a single target into a [health.Checker].
type perTargetChecker struct {
	ph     *poolHealth
	target string
}

func (c *perTargetChecker) CheckHealth(_ context.Context) health.ServingStatus {
	return c.ph.targetStatus(c.target)
}

// perTargetServiceName joins the base service name with a target address.
// The result is opaque to [observability/health.Coordinator] which only
// requires non-empty.
func perTargetServiceName(base, target string) string {
	return corestrings.BuildString(func(b *strings.Builder) {
		b.WriteString(base)
		b.WriteByte('.')
		b.WriteString(target)
	})
}

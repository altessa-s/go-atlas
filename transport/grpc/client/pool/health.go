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

	corestrings "github.com/altessa-s/go-atlas-core/text/strings"
)

// DefaultPoolHealthServiceName is the service name used when the pool
// registers its aggregate checker with a health reporter.
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
// between the [stateTracker] and the user-supplied health reporter. When no
// reporter is configured, the helper still backs [ConnectionPool.CheckHealth]
// and [ConnectionPool.StateForTarget] using the configured mapper.
type poolHealth struct {
	pool        *ConnectionPool
	enabled     bool
	serviceName string
	mapper      StateMapper // never nil
	perTarget   bool

	mu                sync.Mutex
	stopping          bool
	aggregate         targetRegistration
	registeredTargets map[string]targetRegistration
}

func newPoolHealth(p *ConnectionPool) *poolHealth {
	return &poolHealth{
		pool:              p,
		enabled:           p.opts.healthCoordinator != nil || p.opts.scopedHealthReporter != nil,
		serviceName:       p.opts.healthServiceName,
		mapper:            p.opts.healthStateMapper,
		perTarget:         p.opts.healthPerTarget,
		registeredTargets: make(map[string]targetRegistration),
	}
}

type targetRegistration struct {
	count      int
	notify     func(health.ServingStatus)
	unregister func()
}

func (h *poolHealth) registerService(name string, checker health.Checker) targetRegistration {
	if scoped := h.pool.opts.scopedHealthReporter; scoped != nil {
		notify, unregister := scoped.RegisterScopedService(name, checker)
		return targetRegistration{notify: notify, unregister: unregister}
	}
	notify, unregister := health.RegisterOwnedService(h.pool.opts.healthCoordinator, name, checker)
	return targetRegistration{notify: notify, unregister: unregister}
}

func (h *poolHealth) register() {
	if !h.enabled {
		return
	}
	h.mu.Lock()
	h.aggregate = h.registerService(h.serviceName, h)
	h.mu.Unlock()
	h.pool.tracker.enable()
}

func (h *poolHealth) unregister() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopping = true
	for target, reg := range h.registeredTargets {
		reg.unregister()
		delete(h.registeredTargets, target)
	}
	if h.aggregate.unregister != nil {
		h.aggregate.unregister()
		h.aggregate = targetRegistration{}
	}
}

// onTargetStateChange forwards aggregate and per-target connection states.
func (h *poolHealth) onTargetStateChange(target string, _ connectivity.State) {
	if !h.enabled {
		return
	}
	h.mu.Lock()
	aggregate, service := h.aggregate, h.registeredTargets[target]
	h.mu.Unlock()
	if aggregate.notify != nil {
		aggregate.notify(h.aggregateStatus())
	}
	if service.notify != nil {
		service.notify(h.targetStatus(target))
	}
}

// onConnAttached registers a per-target service on its first live connection.
func (h *poolHealth) onConnAttached(target string) {
	if !h.enabled || !h.perTarget {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopping {
		return
	}
	reg := h.registeredTargets[target]
	if reg.count == 0 {
		reg = h.registerService(perTargetServiceName(h.serviceName, target), &perTargetChecker{ph: h, target: target})
	}
	reg.count++
	h.registeredTargets[target] = reg
}

func (h *poolHealth) onConnDetached(target string) {
	if !h.enabled || !h.perTarget {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	reg, ok := h.registeredTargets[target]
	if !ok {
		return
	}
	reg.count--
	if reg.count == 0 {
		reg.unregister()
		delete(h.registeredTargets, target)
		return
	}
	h.registeredTargets[target] = reg
}

// CheckHealth implements [health.Checker] for the aggregate pool service.
func (h *poolHealth) CheckHealth(_ context.Context) health.ServingStatus {
	return h.aggregateStatus()
}

// aggregateStatus returns the worst per-target status across the pool. With
// no tracked targets it returns Serving — the pool is empty but functional.
func (h *poolHealth) aggregateStatus() health.ServingStatus {
	if h.pool.stopped.Load() {
		return health.StatusNotServing
	}
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

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas-core/runtime/singleflight"

	corectx "github.com/altessa-s/go-atlas-core/context"
)

type statusReporter struct {
	registry                  serviceRegistry
	checkTimeout              time.Duration
	maxConcurrentHealthChecks int
	statusCacheTTL            time.Duration
	refresh                   *singleflight.Group[string, ServingStatus]
	override                  atomic.Int32
	listCallsInFlight         atomic.Int32
}

func (s *statusReporter) serviceHealth(ctx context.Context, service string) ServingStatus {
	result := s.checkService(ctx, service)
	if result.unregistered {
		return StatusServiceUnknown
	}
	if result.replaced {
		return StatusNotServing
	}
	return result.status
}

func (s *statusReporter) checkService(ctx context.Context, service string) serviceStatus {
	checker := s.registry.acquire(service)
	if checker == nil {
		return serviceStatus{service: service, status: StatusServiceUnknown, unregistered: true}
	}

	defer checker.checks.Done()

	var status ServingStatus
	if override := ServingStatus(s.override.Load()); override != StatusUnknown {
		status = override
	} else {
		checkCtx, cancel := corectx.WithMaxTimeout(ctx, s.checkTimeout)
		defer cancel()
		status = checker.CheckHealth(checkCtx)
	}

	current := s.registry.lookup(service)
	return serviceStatus{service: service, registration: checker, status: status, unregistered: current == nil, replaced: current != nil && current != checker}
}

func (s *statusReporter) overall(ctx context.Context) ServingStatus {
	if override := ServingStatus(s.override.Load()); override != StatusUnknown {
		return override
	}

	statuses, err := s.checkServices(ctx)
	if err != nil {
		return StatusNotServing
	}

	for _, st := range statuses {
		// A service unregistered while the checks ran is not part of the
		// overall status.
		if !st.unregistered && (st.replaced || st.status != StatusServing) {
			return StatusNotServing
		}
	}

	return StatusServing
}

// serviceStatus is the result of checking one registered service.
type serviceStatus struct {
	unregistered bool
	replaced     bool
	service      string
	registration *serviceRegistration
	status       ServingStatus
}

// checkServices checks every registered service concurrently, bounded by
// MaxConcurrentHealthChecks, each under the check timeout.
func (s *statusReporter) checkServices(ctx context.Context) ([]serviceStatus, error) {
	services := s.registry.names()
	if len(services) == 0 {
		return nil, nil
	}

	statuses, err := concurrency.ProcessCollect(ctx, services, func(ctx context.Context, svc string) (serviceStatus, error) {
		return s.checkService(ctx, svc), nil
	},
		// No StopOnError: continue checking other services if one fails.
		concurrency.WithConcurrency[string](s.maxConcurrentHealthChecks),
	)
	if err != nil {
		return statuses, err
	}

	s.registry.revalidate(statuses)
	return statuses, nil
}

func (s *statusReporter) getCachedStatus(service string) (ServingStatus, bool) {
	if s.statusCacheTTL <= 0 {
		return StatusUnknown, false
	}
	if cached, ok := s.registry.cache.Load(service); ok {
		if cs, ok := cached.(*cachedStatus); ok {
			timestamp := cs.timestamp.Load()
			if time.Since(time.Unix(0, timestamp)) < s.statusCacheTTL {
				return cs.status, true
			}
		}
	}
	return StatusUnknown, false
}

func (s *statusReporter) getHealthStatus(ctx context.Context, service string) ServingStatus {
	// Honor the overall status override before reading the cache so that
	// shutdown signals (see [Coordinator.SetOverallStatus]) cannot be masked
	// by a Serving entry written just before the override was set.
	if override := ServingStatus(s.override.Load()); override != StatusUnknown {
		if service == "" {
			return override
		}
		if s.registry.lookup(service) == nil {
			return StatusServiceUnknown
		}
		return override
	}

	if status, ok := s.getCachedStatus(service); ok {
		return status
	}

	// Callers that find the entry expired at the same time share one run of
	// the checkers, bounded by the check timeout of the group.
	status, err := s.refresh.Do(ctx, service, func(work context.Context) (ServingStatus, error) {
		return s.refreshStatus(work, service), nil
	})
	if errors.Is(err, singleflight.ErrOverloaded) {
		return s.refreshStatus(ctx, service)
	}
	if err != nil {
		// The caller stopped waiting; the shared refresh still fills the cache.
		return StatusNotServing
	}
	return status
}

// refreshStatus runs the checkers behind service and caches the result.
func (s *statusReporter) refreshStatus(ctx context.Context, service string) ServingStatus {
	snapshot := s.registry.snapshot(service)
	var status ServingStatus
	if service == "" {
		status = s.overall(ctx)
	} else {
		status = s.serviceHealth(ctx, service)
	}
	return s.registry.finishRefresh(service, snapshot, status)
}

func (s *statusReporter) list(ctx context.Context) (map[string]ServingStatus, error) {
	s.listCallsInFlight.Add(1)
	defer s.listCallsInFlight.Add(-1)

	statuses, err := s.checkServices(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]ServingStatus, len(statuses))
	for _, r := range statuses {
		if r.unregistered {
			continue
		}
		if r.replaced {
			r.status = StatusNotServing
		}
		results[r.service] = r.status
	}

	return results, nil
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// The registry lock serializes registration changes with cache invalidation,
// publication of a checked result, and delivery of a pushed status to watchers.
type serviceRegistry struct {
	mu       sync.RWMutex
	services map[string]*serviceRegistration
	version  uint64
	cache    sync.Map // map[string]*cachedStatus
}

type serviceRegistration struct {
	Checker
	version uint64 // guarded by serviceRegistry.mu
}

type cachedStatus struct {
	status    ServingStatus
	timestamp atomic.Int64
}

func (r *serviceRegistry) register(name string, checker Checker) *serviceRegistration {
	reg := &serviceRegistration{Checker: checker}
	r.mu.Lock()
	r.services[name] = reg
	r.version++
	r.cache.Delete(name)
	r.cache.Delete("")
	r.mu.Unlock()
	return reg
}

func (r *serviceRegistry) remove(name string, owner *serviceRegistration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if owner != nil && r.services[name] != owner {
		return
	}
	delete(r.services, name)
	r.version++
	r.cache.Delete(name)
	r.cache.Delete("")
}

func (r *serviceRegistry) names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Collect(maps.Keys(r.services))
}

func (r *serviceRegistry) lookup(name string) *serviceRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.services[name]
}

func (r *serviceRegistry) report(service string, owner *serviceRegistration, status ServingStatus, watchers *subscriptions) {
	r.mu.Lock()
	registration := r.services[service]
	if owner != nil && registration != owner {
		r.mu.Unlock()
		return
	}
	r.version++
	if registration != nil {
		registration.version++
	}
	if service != "" {
		r.cache.Delete("")
	}
	r.setCachedStatus(service, status)
	count := watchers.notifyWatchers(service, status)
	r.mu.Unlock()
	watchers.logStatusChange(service, status, count)
}

func (r *serviceRegistry) setCachedStatus(service string, status ServingStatus) {
	cs := &cachedStatus{status: status}
	cs.timestamp.Store(time.Now().UnixNano())
	r.cache.Store(service, cs)
}

// Each refresh records both versions under one lock. A named check ignores
// unrelated changes, while an aggregate check depends on the whole registry.
type registrationSnapshot struct {
	aggregateVersion uint64
	reg              *serviceRegistration
	version          uint64
}

func (r *serviceRegistry) snapshot(service string) registrationSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot := registrationSnapshot{aggregateVersion: r.version, reg: r.services[service]}
	if snapshot.reg != nil {
		snapshot.version = snapshot.reg.version
	}
	return snapshot
}

func (r *serviceRegistry) finishRefresh(service string, snapshot registrationSnapshot, status ServingStatus) ServingStatus {
	// Unknown names come from callers (HTTP and gRPC health requests);
	// caching them would retain every distinct name ever asked for.
	r.mu.RLock()
	defer r.mu.RUnlock()
	current := snapshot.aggregateVersion == r.version
	if service != "" {
		current = snapshot.reg != nil && r.services[service] == snapshot.reg && snapshot.reg.version == snapshot.version
	}
	if !current {
		// A push can supersede a shared refresh before another caller joins it.
		if latest, ok := r.cache.Load(service); ok {
			return latest.(*cachedStatus).status //nolint:forcetypeassert,errcheck // statusCache stores only *cachedStatus.
		}
	} else if status != StatusServiceUnknown {
		r.setCachedStatus(service, status)
	}
	return status
}

func (r *serviceRegistry) revalidate(statuses []serviceStatus) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range statuses {
		current, registered := r.services[statuses[i].service]
		statuses[i].unregistered = !registered
		statuses[i].replaced = registered && current != statuses[i].registration
	}
}

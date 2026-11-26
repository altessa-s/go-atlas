// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"sync"
)

// DetachableChecker gives a checker a lifetime even when it is registered
// through the original [Registrar] interface.
type DetachableChecker struct {
	mu       sync.Mutex
	checker  Checker
	bindings []serviceBinding
}

// NewDetachableChecker wraps checker with an independently controlled lifetime.
func NewDetachableChecker(checker Checker) *DetachableChecker {
	return &DetachableChecker{checker: checker}
}

// CheckHealth delegates to the checker until it is detached.
func (c *DetachableChecker) CheckHealth(ctx context.Context) ServingStatus {
	c.mu.Lock()
	checker := c.checker
	c.mu.Unlock()
	if checker == nil {
		return StatusServiceUnknown
	}
	return checker.CheckHealth(ctx)
}

// Detach releases the underlying checker and removes its registrations with
// [Coordinator]. It is safe to call repeatedly.
func (c *DetachableChecker) Detach() { c.detach(nil) }

func (c *DetachableChecker) detach(unregisterLegacy func()) {
	c.mu.Lock()
	if c.checker == nil {
		c.mu.Unlock()
		return
	}
	c.checker = nil
	bindings := c.bindings
	c.bindings = nil
	c.mu.Unlock()
	if unregisterLegacy != nil && bindingsCurrent(bindings) {
		unregisterLegacy()
	}
	for _, binding := range bindings {
		binding.remove()
	}
}

type serviceBinding struct {
	registry *serviceRegistry
	name     string
	owner    *serviceRegistration
}

func (b serviceBinding) remove() { b.registry.remove(b.name, b.owner) }

func bindingsCurrent(bindings []serviceBinding) bool {
	for _, b := range bindings {
		if b.registry.lookup(b.name) != b.owner {
			return false
		}
	}
	return true
}

func (c *DetachableChecker) bind(binding serviceBinding) {
	c.mu.Lock()
	if c.checker != nil {
		c.bindings = append(c.bindings, binding)
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	binding.remove()
}

// RegisterOwnedService registers through the supplied Reporter, including its
// method overrides. A direct Coordinator uses its scoped registration. A
// legacy reporter receives name-based updates and, for DynamicReporter,
// teardown. Use ScopedReporter explicitly to serialize these with replacement.
func RegisterOwnedService(reporter Reporter, name string, checker Checker) (notify func(ServingStatus), unregister func()) {
	if coordinator, ok := reporter.(*Coordinator); ok {
		return coordinator.RegisterScopedService(name, checker)
	}
	detachable, unregister := registerDetached(reporter, name, checker)
	return func(status ServingStatus) {
		detachable.mu.Lock()
		active := detachable.checker != nil
		bindings := detachable.bindings
		detachable.mu.Unlock()
		if active && bindingsCurrent(bindings) {
			reporter.NotifyStatusChange(name, status)
		}
	}, unregister
}

// RegisterOwnedChecker registers a checker for the lifetime of its owner.
// Registrars that provide scoped registration remove the checker on cleanup.
// Other registrars can support name-based removal through [DynamicReporter].
func RegisterOwnedChecker(registrar Registrar, name string, checker Checker) func() {
	if scoped, ok := registrar.(ScopedReporter); ok {
		_, unregister := scoped.RegisterScopedService(name, checker)
		return unregister
	}
	_, unregister := registerDetached(registrar, name, checker)
	return unregister
}

func registerDetached(registrar Registrar, name string, checker Checker) (*DetachableChecker, func()) {
	detachable := NewDetachableChecker(checker)
	registrar.RegisterService(name, detachable)
	var unregisterLegacy func()
	if dynamic, ok := registrar.(DynamicReporter); ok {
		unregisterLegacy = func() { dynamic.UnregisterService(name) }
	}
	return detachable, func() { detachable.detach(unregisterLegacy) }
}

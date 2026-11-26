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

// Detach releases the underlying checker and removes its bound registrations.
// It is safe to call repeatedly.
func (c *DetachableChecker) Detach() {
	c.mu.Lock()
	if c.checker == nil {
		c.mu.Unlock()
		return
	}
	c.checker = nil
	bindings := c.bindings
	c.bindings = nil
	c.mu.Unlock()
	for _, binding := range bindings {
		binding.remove()
	}
}

type serviceBinding struct {
	remove  func()
	current func() bool
}

func bindingsCurrent(bindings []serviceBinding) bool {
	for _, b := range bindings {
		if b.current != nil && !b.current() {
			return false
		}
	}
	return true
}

// BindRemoval links a registrar's own entry to the checker's lifetime. A
// registrar that stores the checker should bind a callback that removes only
// the entry created by that registration, even when several entries share a
// name. Call BindRemoval outside the registrar's lock. If the checker was
// already detached, BindRemoval runs the callback before returning.
func (c *DetachableChecker) BindRemoval(remove func()) {
	c.bind(serviceBinding{remove: remove})
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

// RegisterOwnedService prefers scoped reporting for replacement-safe updates
// and removal. A legacy reporter receives name-based updates and can bind
// removal of its own entry via [DetachableChecker.BindRemoval].
func RegisterOwnedService(reporter Reporter, name string, checker Checker) (notify func(ServingStatus), unregister func()) {
	if scoped, ok := reporter.(ScopedReporter); ok {
		return scoped.RegisterScopedService(name, checker)
	}
	detachable := NewDetachableChecker(checker)
	reporter.RegisterService(name, detachable)
	return func(status ServingStatus) {
		detachable.mu.Lock()
		active := detachable.checker != nil
		bindings := detachable.bindings
		detachable.mu.Unlock()
		if active && bindingsCurrent(bindings) {
			reporter.NotifyStatusChange(name, status)
		}
	}, detachable.Detach
}

// RegisterOwnedChecker registers a checker for the lifetime of its owner.
// Registrars can support cleanup by implementing [ScopedReporter] or binding
// entry removal via [DetachableChecker.BindRemoval].
func RegisterOwnedChecker(registrar Registrar, name string, checker Checker) func() {
	if scoped, ok := registrar.(ScopedReporter); ok {
		_, unregister := scoped.RegisterScopedService(name, checker)
		return unregister
	}
	detachable := NewDetachableChecker(checker)
	registrar.RegisterService(name, detachable)
	return detachable.Detach
}

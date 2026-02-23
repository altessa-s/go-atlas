// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"sync"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
)

// ErrFilterNotFound is returned when a filter is not registered.
var ErrFilterNotFound = errors.New("filter not found")

// ErrFilterAlreadyExists is returned when a filter name is already registered.
var ErrFilterAlreadyExists = errors.New("filter already exists")

// Manager provides a facade for managing multiple named probabilistic filters.
// It is safe for concurrent use.
type Manager struct {
	filters map[string]Filter
	mu      sync.RWMutex
	opts    *options
}

// NewManager creates a new Manager for managing multiple filters.
//
// Example:
//
//	mgr := probfilter.NewManager()
//	mgr.Register("users", userFilter)
//	mgr.Register("sessions", sessionFilter)
func NewManager(opt ...Option) *Manager {
	return &Manager{
		filters: make(map[string]Filter),
		opts:    newOptions(opt...),
	}
}

// Register adds a named filter to the manager.
// Returns ErrFilterAlreadyExists if a filter with the same name is already registered.
func (m *Manager) Register(name string, filter Filter) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.filters[name]; exists {
		return ErrFilterAlreadyExists
	}

	m.filters[name] = filter
	return nil
}

// Get retrieves a filter by name.
// Returns ErrFilterNotFound if the filter is not registered.
func (m *Manager) Get(name string) (Filter, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filter, exists := m.filters[name]
	if !exists {
		return nil, ErrFilterNotFound
	}

	return filter, nil
}

// MustGet retrieves a filter by name or panics if not found.
func (m *Manager) MustGet(name string) Filter {
	return panics.MustResult(m.Get(name))
}

// Unregister removes a filter from the manager.
// The filter is not closed; the caller is responsible for closing it.
func (m *Manager) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.filters, name)
}

// Names returns an iterator over the names of all registered filters.
func (m *Manager) Names() iter.Seq[string] {
	return func(yield func(string) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		for name := range m.filters {
			if !yield(name) {
				return
			}
		}
	}
}

// Filters returns an iterator over the names and filters registered in the manager.
func (m *Manager) Filters() iter.Seq2[string, Filter] {
	return func(yield func(string, Filter) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		for name, filter := range m.filters {
			if !yield(name, filter) {
				return
			}
		}
	}
}

// Close closes all registered filters that implement io.Closer and clears the manager.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for name, filter := range m.filters {
		if closer, ok := filter.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
		}
	}

	m.filters = make(map[string]Filter)

	return errors.Join(errs...)
}

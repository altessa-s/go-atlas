// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter

import (
	"context"
	"errors"
	"io"
	"iter"
	"sync"

	"github.com/altessa-s/go-atlas/core/runtime/panics"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrFilterNotFound is returned when a filter is not registered.
var ErrFilterNotFound = errors.New("filter not found")

// ErrFilterAlreadyExists is returned when a filter name is already registered.
var ErrFilterAlreadyExists = errors.New("filter already exists")

// Manager provides a facade for managing multiple named probabilistic filters.
// It is safe for concurrent use.
//
// When created with [WithCollector], the Manager records the probfilter
// metrics for every registered filter that implements [ObservableFilter]
// (the Bloom and Cuckoo facades do), labeled with the registration name.
type Manager struct {
	filters map[string]Filter
	mu      sync.RWMutex
	opts    *options
	metrics *probfilterMetrics
}

// contextCloser is implemented by filters whose Close takes a context, such
// as the Bloom and Cuckoo facades.
type contextCloser interface {
	Close(ctx context.Context) error
}

// NewManager creates a new Manager for managing multiple filters.
//
// Example:
//
//	mgr := probfilter.NewManager()
//	mgr.Register("users", userFilter)
//	mgr.Register("sessions", sessionFilter)
func NewManager(opt ...Option) *Manager {
	opts := newOptions(opt...)
	return &Manager{
		filters: make(map[string]Filter),
		opts:    opts,
		metrics: newProbfilterMetrics(opts.collector),
	}
}

// Register adds a named filter to the manager.
// Returns ErrFilterAlreadyExists if a filter with the same name is already registered.
// When the Manager has a metrics collector and filter implements
// [ObservableFilter], Register attaches an observer recording its operations
// under name; a filter registered in several places reports to the last one.
func (m *Manager) Register(name string, filter Filter) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.filters[name]; exists {
		return ErrFilterAlreadyExists
	}

	m.filters[name] = filter
	if of, ok := filter.(ObservableFilter); ok && m.opts.collector != nil {
		of.SetObserver(m.metrics.observer(name))
	}
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

// Unregister removes a filter from the manager and detaches the metrics
// observer attached by [Manager.Register].
// The filter is not closed; the caller is responsible for closing it.
func (m *Manager) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if filter, ok := m.filters[name]; ok {
		detachObserver(m.opts, filter)
	}
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

// Close closes all registered filters that implement io.Closer or expose
// Close(context.Context) error (the Bloom and Cuckoo facades), and clears the
// manager.
//
// The filters are detached under the lock and closed after releasing it:
// closing a filter waits for its running rebuild, whose loader may itself
// read the Manager.
func (m *Manager) Close() error {
	m.mu.Lock()
	filters := m.filters
	m.filters = make(map[string]Filter)
	m.mu.Unlock()

	var errs []error
	for name, filter := range filters {
		detachObserver(m.opts, filter)

		var err error
		switch closer := filter.(type) {
		case io.Closer:
			err = closer.Close()
		case contextCloser:
			err = closer.Close(context.Background())
		}
		if err != nil {
			errs = append(errs, coreerrs.Wrap(err, name))
		}
	}

	return errors.Join(errs...)
}

// detachObserver removes the observer Register attached to filter.
func detachObserver(opts *options, filter Filter) {
	if of, ok := filter.(ObservableFilter); ok && opts.collector != nil {
		of.SetObserver(nil)
	}
}

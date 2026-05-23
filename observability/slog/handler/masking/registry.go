// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

import (
	"fmt"
	"sync"
)

// Registry stores registered mask functions.
// It provides thread-safe access to mask functions and factories.
type Registry struct {
	mu    sync.RWMutex
	funcs map[string]MaskFunc
}

// MaskFactory creates a MaskFunc with parameters.
// Used for parameterized masks like partial, fixed, pattern.
type MaskFactory func(params map[string]any) (MaskFunc, error)

var (
	// defaultRegistry is the global mask function registry.
	defaultRegistry = &Registry{
		funcs: make(map[string]MaskFunc),
	}

	// factories stores factories for parameterized masks.
	factories   = make(map[string]MaskFactory)
	factoriesMu sync.RWMutex
)

// Register registers a mask function under the given name in the global registry.
// This is typically called from init() functions to register built-in masks.
//
// Example:
//
//	func init() {
//	    masking.Register("email", EmailMask())
//	}
func Register(name string, fn MaskFunc) {
	defaultRegistry.Register(name, fn)
}

// Get returns a mask function by name from the global registry.
// Returns false if the mask is not found.
func Get(name string) (MaskFunc, bool) {
	return defaultRegistry.Get(name)
}

// List returns the names of all registered mask functions.
// Useful for documentation and validation.
func List() []string {
	return defaultRegistry.List()
}

// RegisterFactory registers a factory for parameterized masks.
// Factories are used for masks that require configuration parameters.
//
// Example:
//
//	masking.RegisterFactory("partial", func(params map[string]any) (MaskFunc, error) {
//	    showFirst, _ := params["showFirst"].(int)
//	    showLast, _ := params["showLast"].(int)
//	    return PartialMask(showFirst, showLast, "*"), nil
//	})
func RegisterFactory(name string, factory MaskFactory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[name] = factory
}

// GetFactory returns a mask factory by name.
// Returns false if the factory is not found.
func GetFactory(name string) (MaskFactory, bool) {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	f, ok := factories[name]
	return f, ok
}

// ListFactories returns the names of all registered factories.
func ListFactories() []string {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()

	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	return names
}

// CreateMask creates a mask function by name, with optional parameters.
// It first checks for simple masks, then factories if parameters are provided.
func CreateMask(name string, params map[string]any) (MaskFunc, error) {
	// Check simple masks first (if no params)
	if len(params) == 0 {
		if fn, ok := Get(name); ok {
			return fn, nil
		}
	}

	// Check factories
	if factory, ok := GetFactory(name); ok {
		return factory(params)
	}

	// If it's a simple mask but params were provided
	if _, ok := Get(name); ok && len(params) > 0 {
		return nil, fmt.Errorf("mask %q does not accept parameters", name)
	}

	return nil, fmt.Errorf("unknown mask type: %q", name)
}

// Register registers a mask function in the registry.
func (r *Registry) Register(name string, fn MaskFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.funcs[name] = fn
}

// Get returns a mask function by name.
func (r *Registry) Get(name string) (MaskFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.funcs[name]
	return fn, ok
}

// List returns all registered mask names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.funcs))
	for name := range r.funcs {
		names = append(names, name)
	}
	return names
}

// Clear removes all registered masks (useful for testing).
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.funcs = make(map[string]MaskFunc)
}

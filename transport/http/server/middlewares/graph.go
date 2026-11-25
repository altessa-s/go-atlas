// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"log/slog"
	"slices"

	"github.com/altessa-s/go-atlas/internal/transport/depgraph"
)

// middlewareWrapper wraps a Middleware to satisfy depgraph.Namer
// and forward DependencyDeclarer.
type middlewareWrapper struct {
	middleware Middleware
	outer      []string
}

// Name implements depgraph.Namer.
func (w middlewareWrapper) Name() string {
	return w.middleware.Name()
}

// Dependencies implements depgraph.DependencyDeclarer by forwarding to the underlying middleware.
func (w middlewareWrapper) Dependencies() []string {
	if declarer, ok := w.middleware.(depgraph.DependencyDeclarer); ok {
		if len(w.outer) == 0 {
			return declarer.Dependencies()
		}
		return slices.Concat(w.outer, declarer.Dependencies())
	}
	return w.outer
}

// RequiredDependencies implements depgraph.RequiredDependencyDeclarer by forwarding to the underlying middleware.
func (w middlewareWrapper) RequiredDependencies() []string {
	if declarer, ok := w.middleware.(depgraph.RequiredDependencyDeclarer); ok {
		return declarer.RequiredDependencies()
	}
	return nil
}

// orderByDependencies sorts middlewares using dependency-based topological sort.
// Returns an error if a circular dependency is detected.
func orderByDependencies(items []Middleware, logger *slog.Logger) ([]Middleware, error) {
	if len(items) == 0 {
		return items, nil
	}

	// Every ordinary middleware follows the outer group. Let the dependency
	// graph reject conflicting constraints just like other cycles.
	var outer []string
	for _, m := range items {
		if o, ok := m.(OutermostMiddleware); ok && o.Outermost() {
			outer = append(outer, m.Name())
		}
	}
	wrapped := make([]middlewareWrapper, len(items))
	for i, m := range items {
		wrapped[i] = middlewareWrapper{middleware: m}
		if len(outer) > 0 && !slices.Contains(outer, m.Name()) {
			wrapped[i].outer = outer
		}
	}

	// Sort using shared depgraph
	sorted, err := depgraph.Build(wrapped, logger)
	if err != nil {
		return nil, err
	}

	// Unwrap results
	result := make([]Middleware, len(sorted))
	for i, w := range sorted {
		result[i] = w.middleware
	}

	return result, nil
}

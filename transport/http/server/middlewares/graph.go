// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/internal/depgraph"
)

// middlewareWrapper wraps a Middleware to satisfy depgraph.Namer
// and forward DependencyDeclarer.
type middlewareWrapper struct {
	middleware Middleware
}

// Name implements depgraph.Namer.
func (w middlewareWrapper) Name() string {
	return w.middleware.Name()
}

// Dependencies implements depgraph.DependencyDeclarer by forwarding to the underlying middleware.
func (w middlewareWrapper) Dependencies() []string {
	if declarer, ok := w.middleware.(depgraph.DependencyDeclarer); ok {
		return declarer.Dependencies()
	}
	return nil
}

// orderByDependencies sorts middlewares using dependency-based topological sort.
// Returns an error if a circular dependency is detected.
func orderByDependencies(items []Middleware, logger *slog.Logger) ([]Middleware, error) {
	if len(items) == 0 {
		return items, nil
	}

	// Wrap middlewares for depgraph
	wrapped := make([]middlewareWrapper, len(items))
	for i, m := range items {
		wrapped[i] = middlewareWrapper{middleware: m}
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

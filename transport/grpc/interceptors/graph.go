// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/internal/depgraph"
)

// namedItem wraps an interceptor item for use with the generic depgraph package.
// It adapts items that implement Interceptor to the depgraph.Namer interface
// and forwards DependencyDeclarer to the underlying item.
type namedItem struct {
	item any
	name string
}

// Name implements depgraph.Namer.
func (n namedItem) Name() string { return n.name }

// Dependencies implements depgraph.DependencyDeclarer by forwarding to the underlying item.
func (n namedItem) Dependencies() []string {
	if declarer, ok := n.item.(depgraph.DependencyDeclarer); ok {
		return declarer.Dependencies()
	}
	return nil
}

// wrapItems converts a slice of any items to namedItem slice for depgraph.
func wrapItems(items []any) []namedItem {
	result := make([]namedItem, len(items))
	for i, item := range items {
		result[i] = namedItem{
			item: item,
			name: getInterceptorName(item),
		}
	}
	return result
}

// unwrapItems converts a slice of namedItem back to []any.
func unwrapItems(items []namedItem) []any {
	result := make([]any, len(items))
	for i, item := range items {
		result[i] = item.item
	}
	return result
}

// getInterceptorName extracts the name from an interceptor.
func getInterceptorName(item any) string {
	if named, ok := item.(Interceptor); ok {
		return named.Name()
	}
	return ""
}

// orderByDependencies sorts interceptors using dependency-based topological sort.
// Returns an error if a circular dependency is detected.
func orderByDependencies(items []any, logger *slog.Logger) ([]any, error) {
	if len(items) == 0 {
		return items, nil
	}

	wrapped := wrapItems(items)
	sorted, err := depgraph.Build(wrapped, logger)
	if err != nil {
		return nil, err
	}
	return unwrapItems(sorted), nil
}

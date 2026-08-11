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

// RequiredDependencies implements depgraph.RequiredDependencyDeclarer by forwarding to the underlying item.
func (n namedItem) RequiredDependencies() []string {
	if declarer, ok := n.item.(depgraph.RequiredDependencyDeclarer); ok {
		return declarer.RequiredDependencies()
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

// dedupeByName returns items with every repeated interceptor name removed,
// keeping the first occurrence and the original order. Items that report no
// name are never treated as duplicates of one another — there is nothing to
// compare them by.
//
// It runs before [depgraph.Build], not after. Build refuses a repeated name
// rather than choosing between the two, so discarding one has to be the
// caller's deliberate act; a pass that ran afterwards could only confirm a
// discard the graph had already made on its own.
func dedupeByName(items []any) []any {
	if len(items) < 2 { //nolint:mnd // a list shorter than two cannot repeat.
		return items
	}

	seen := make(map[string]struct{}, len(items))
	out := make([]any, 0, len(items))
	for _, item := range items {
		name := getInterceptorName(item)
		if name != "" {
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
		}
		out = append(out, item)
	}
	return out
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

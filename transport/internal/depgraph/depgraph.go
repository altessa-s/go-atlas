// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package depgraph

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Namer is implemented by graph items that can identify themselves by a
// unique name. The name is used as the node key and in cycle-detection
// error messages.
type Namer interface {
	Name() string
}

// DependencyDeclarer declares dependencies for automatic ordering.
// Items implementing this interface will be automatically ordered
// based on their declared dependencies using topological sort.
//
// All dependencies are optional for ordering purposes. If a declared
// dependency is missing from the graph, it is silently ignored and
// logged at Debug level. The item will still function correctly
// without its dependencies.
type DependencyDeclarer interface {
	// Dependencies returns names of items that should run before this one.
	// If a dependency is missing from the graph, it's silently ignored.
	// Return nil or empty slice for items with no dependencies.
	Dependencies() []string
}

// RequiredDependencyDeclarer declares dependencies that MUST be present.
// If any required dependency is missing from the graph, Build returns an error.
// Required dependencies also impose ordering (like DependencyDeclarer).
type RequiredDependencyDeclarer interface {
	RequiredDependencies() []string
}

// Graph is a directed acyclic graph for dependency resolution.
// It is not safe for concurrent use; callers must synchronize externally
// or build the graph in a single goroutine before calling [Graph.TopologicalSort].
type Graph[T Namer] struct {
	// nodes maps item name to its value
	nodes map[string]T
	// edges maps item name to its dependents (items that depend on it)
	edges map[string][]string
	// inDegree tracks incoming edge count for each node
	inDegree map[string]int
	// order tracks insertion order for stable sorting
	order map[string]int
	// logger for debug output
	logger *slog.Logger
}

// New creates a new dependency graph.
func New[T Namer](logger *slog.Logger) *Graph[T] {
	return &Graph[T]{
		nodes:    make(map[string]T),
		edges:    make(map[string][]string),
		inDegree: make(map[string]int),
		order:    make(map[string]int),
		logger:   logger,
	}
}

// AddNode registers an item under the given name. Duplicate names are
// silently ignored, keeping the first registration.
func (g *Graph[T]) AddNode(name string, item T) {
	if _, exists := g.nodes[name]; !exists {
		g.order[name] = len(g.nodes)
		g.nodes[name] = item
		g.inDegree[name] = 0
	}
}

// HasNode checks if a node exists in the graph.
func (g *Graph[T]) HasNode(name string) bool {
	_, exists := g.nodes[name]
	return exists
}

// AddEdge records that dep must be ordered before dependent. Both nodes
// should already exist in the graph; adding edges for unknown nodes will
// cause TopologicalSort to report a cycle.
func (g *Graph[T]) AddEdge(dep, dependent string) {
	g.edges[dep] = append(g.edges[dep], dependent)
	g.inDegree[dependent]++
}

// TopologicalSort returns all items ordered so that every dependency
// appears before its dependents (Kahn's algorithm). Ties are broken by
// insertion order for deterministic results.
//
// Returns an error listing the involved nodes when a cycle is detected.
func (g *Graph[T]) TopologicalSort() ([]T, error) {
	if len(g.nodes) == 0 {
		return nil, nil
	}

	// Find all nodes with no incoming edges
	queue := make([]string, 0, len(g.nodes))
	for name := range g.nodes {
		queue = coreslices.AppendIf(queue, g.inDegree[name] == 0, name)
	}

	// Sort initial queue by insertion order for stability
	slices.SortStableFunc(queue, func(a, b string) int {
		return g.order[a] - g.order[b]
	})

	result := make([]T, 0, len(g.nodes))
	inDegree := maps.Clone(g.inDegree)

	for len(queue) > 0 {
		// Process first item
		name := queue[0]
		queue = queue[1:]

		result = append(result, g.nodes[name])

		// Process dependents
		dependents := g.edges[name]
		var newZero []string

		for _, dep := range dependents {
			inDegree[dep]--
			newZero = coreslices.AppendIf(newZero, inDegree[dep] == 0, dep)
		}

		// Merge new zero-degree nodes and re-sort entire queue by insertion
		// order so that earlier-registered nodes are always dequeued first,
		// regardless of when they became zero-degree ("priority queue" fix).
		if len(newZero) > 0 {
			queue = append(queue, newZero...)
			slices.SortStableFunc(queue, func(a, b string) int {
				return g.order[a] - g.order[b]
			})
		}
	}

	// Check for cycle
	if len(result) != len(g.nodes) {
		// Find nodes in cycle for error message
		var cycleNodes []string
		for name := range g.nodes {
			cycleNodes = coreslices.AppendIf(cycleNodes, inDegree[name] > 0, name)
		}
		return nil, fmt.Errorf("circular dependency detected involving: %v", cycleNodes)
	}

	return result, nil
}

// Build is a convenience function that constructs a [Graph] from items,
// wires edges for every item implementing [DependencyDeclarer] or
// [RequiredDependencyDeclarer], and returns the topologically sorted result.
// Optional dependencies referencing names not present in items are silently
// skipped (logged at Debug level). Required dependencies that are missing
// cause an immediate error.
//
// Returns an error when a required dependency is missing or a circular
// dependency is detected.
func Build[T Namer](items []T, logger *slog.Logger) ([]T, error) {
	if len(items) == 0 {
		return items, nil
	}

	graph := New[T](logger)

	// First pass: add all nodes
	for _, item := range items {
		name := item.Name()
		graph.AddNode(name, item)
	}

	// Second pass: add edges for required and optional dependencies.
	// Required deps are checked first (must exist); optional deps are
	// skipped if missing. Edges already added by required deps are not
	// duplicated when the same name appears in optional deps.
	for _, item := range items {
		name := item.Name()
		var added map[string]struct{}

		if reqDeclarer, ok := any(item).(RequiredDependencyDeclarer); ok {
			for _, dep := range reqDeclarer.RequiredDependencies() {
				if !graph.HasNode(dep) {
					return nil, fmt.Errorf("item %q requires dependency %q which is not registered", name, dep)
				}
				graph.AddEdge(dep, name)
				if added == nil {
					added = make(map[string]struct{})
				}
				added[dep] = struct{}{}
			}
		}

		if declarer, ok := any(item).(DependencyDeclarer); ok {
			for _, dep := range declarer.Dependencies() {
				if _, dup := added[dep]; dup {
					continue
				}
				if graph.HasNode(dep) {
					graph.AddEdge(dep, name)
				} else if logger != nil {
					logger.Debug("dependency not in graph, ignoring",
						slog.String("item", name),
						slog.String("dependency", dep))
				}
			}
		}
	}

	sorted, err := graph.TopologicalSort()
	if err != nil {
		return nil, coreerrs.Wrap(err, "dependency ordering failed")
	}

	return sorted, nil
}

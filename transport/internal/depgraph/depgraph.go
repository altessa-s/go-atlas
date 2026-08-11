// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package depgraph

import (
	"errors"
	"log/slog"
	"maps"
	"slices"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Sentinel errors returned by [Build] and [Graph.TopologicalSort]. They are
// matchable with errors.Is so a caller can tell a misconfiguration it can fix
// (a duplicate or a missing dependency) from one it cannot (a cycle).
var (
	// ErrEmptyName is returned by [Build] when an item reports no name. The
	// graph is keyed by name: an unnamed item cannot be depended upon, cannot
	// be told apart from another unnamed one, and would be dropped rather than
	// ordered. Refusing it names the misconfiguration at startup instead of
	// leaving a silently shorter chain.
	ErrEmptyName = errors.New("depgraph: item has no name")

	// ErrDuplicateName is returned by [Build] when two items report the same
	// name. A graph keyed by name cannot order them: an edge declaring
	// "after limiter" names one node, and with two of them the declaration has
	// no meaning. Deduplicate deliberately before ordering if that is the
	// intent — silently keeping one and discarding the other would drop an
	// item the caller registered.
	ErrDuplicateName = errors.New("depgraph: duplicate item name")

	// ErrMissingDependency is returned by [Build] when an item declares a
	// required dependency that is not among the items.
	ErrMissingDependency = errors.New("depgraph: required dependency not registered")

	// ErrCyclicDependency is returned when the declared dependencies cannot be
	// ordered because they form a cycle.
	ErrCyclicDependency = errors.New("depgraph: circular dependency")
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
//
// This is the low-level primitive; [Build] rejects duplicates with
// [ErrDuplicateName] rather than letting one silently win, because a caller
// that assembled a list does not expect ordering it to shorten the list.
func (g *Graph[T]) AddNode(name string, item T) {
	if _, exists := g.nodes[name]; !exists {
		g.order[name] = len(g.nodes)
		g.nodes[name] = item
		g.inDegree[name] = 0
	}
}

// Dedupe returns items with every repeated name removed, keeping the first
// occurrence and the original order. Unnamed items are left alone: there is
// nothing to compare them by, and collapsing them would discard distinct items
// silently — the failure this whole contract exists to prevent. [Build] refuses
// them outright, so Dedupe must not swallow the evidence first.
//
// [Build] refuses a repeated name rather than choosing between the two, so a
// caller whose input may legitimately contain one — a list assembled from
// several sources, or a default prepended to a user-supplied set — runs it
// through Dedupe first. Doing so before Build rather than after is what makes
// the discard deliberate: afterwards, the graph has already dropped the
// duplicate and a second pass can only confirm what it cannot see.
func Dedupe[T Namer](items []T) []T {
	if len(items) < 2 { //nolint:mnd // a list shorter than two cannot repeat.
		return items
	}

	seen := make(map[string]struct{}, len(items))
	out := make([]T, 0, len(items))
	for _, item := range items {
		name := item.Name()
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
		return nil, coreerrs.Wrapf(ErrCyclicDependency, "involving: %v", cycleNodes)
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
// Every item must report a non-empty, distinct name: ordering is keyed by name,
// so an unnamed item cannot be placed and two items sharing one name cannot
// both be.
//
// Returns [ErrEmptyName] when an item has no name, [ErrDuplicateName] when two
// items share one, [ErrMissingDependency]
// when a required dependency is absent, and [ErrCyclicDependency] when the
// declared dependencies form a cycle.
func Build[T Namer](items []T, logger *slog.Logger) ([]T, error) {
	if len(items) == 0 {
		return items, nil
	}

	graph := New[T](logger)

	// First pass: add all nodes.
	//
	// A repeated name is refused rather than collapsed. The graph is keyed by
	// name, so a second item under an existing one would simply not be added,
	// and the sorted result would come back shorter than the input — an item
	// the caller registered, silently absent from the chain it was ordering.
	for i, item := range items {
		name := item.Name()
		if name == "" {
			return nil, coreerrs.Wrapf(ErrEmptyName, "item at index %d", i)
		}
		if graph.HasNode(name) {
			return nil, coreerrs.Wrapf(ErrDuplicateName, "%q is registered more than once", name)
		}
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
					return nil, coreerrs.Wrapf(ErrMissingDependency,
						"item %q requires dependency %q which is not registered", name, dep)
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

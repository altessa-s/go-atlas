// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package depgraph resolves ordering dependencies among interceptors and
// middlewares using topological sort (Kahn's algorithm).
//
// Items added to a [Graph] must implement [Namer]. Those that also implement
// [DependencyDeclarer] declare which other items must precede them. Missing
// dependencies are silently ignored (logged at Debug level), so the graph
// tolerates partial registration.
//
// When multiple items share the same in-degree, insertion order is preserved
// to provide deterministic output across runs.
//
// [Build] is the high-level entry point:
//
//	sorted, err := depgraph.Build(items, logger)
//	if err != nil {
//	    // circular dependency
//	}
//
// [Graph] is NOT safe for concurrent use; build it in a single goroutine
// before calling [Graph.TopologicalSort].
package depgraph

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import "github.com/altessa-s/go-atlas/transport/internal/depgraph"

// OrderMiddlewares sorts middlewares by their declared dependencies using
// topological sort. Middlewares implementing depgraph.DependencyDeclarer
// declare which other middleware names must precede them. Items without
// dependencies or at the same level maintain their relative position
// (stable sort).
//
// Every middleware must report a distinct name: ordering is keyed by name, and
// a dependency declaring "after limiter" cannot say which of two limiters it
// means. A repeat therefore returns [depgraph.ErrDuplicateName] rather than
// silently dropping one — use [OrderMiddlewaresWithDedupe] when discarding the
// later occurrence is what you actually want.
//
// Returns an error if a required dependency is missing, a name repeats, or a
// circular dependency is detected. See also [Chain.Ordered].
//
// Example:
//
//	middlewares := []Middleware{limiterMiddleware, loggerMiddleware, recoveryMiddleware}
//	ordered, err := middlewares.OrderMiddlewares(middlewares)
//	// ordered: [loggerMiddleware, recoveryMiddleware, limiterMiddleware]
func OrderMiddlewares(middlewares []Middleware) ([]Middleware, error) {
	return orderByDependencies(middlewares, nil)
}

// OrderMiddlewaresWithDedupe removes duplicates by name, keeping the first
// occurrence, and then sorts the remainder by dependencies like
// [OrderMiddlewares]. Use it when the list is assembled from several sources
// and a repeat is expected rather than a mistake.
//
// The deduplication runs before the ordering, which is what makes the discard
// this function's own act: ordering is keyed by name and refuses a repeat, so a
// pass that ran afterwards could only confirm a decision it never made.
//
// Returns an error if a required dependency is missing or a circular dependency
// is detected. See also [Chain.OrderedWithDedupe].
//
// Example:
//
//	middlewares := []Middleware{limiter1, logger, limiter2}
//	ordered, err := middlewares.OrderMiddlewaresWithDedupe(middlewares)
//	// ordered: [logger, limiter1] (limiter2 removed as duplicate)
func OrderMiddlewaresWithDedupe(middlewares []Middleware) ([]Middleware, error) {
	return orderByDependencies(depgraph.Dedupe(middlewares), nil)
}

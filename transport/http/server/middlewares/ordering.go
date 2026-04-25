// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

// OrderMiddlewares sorts middlewares by their declared dependencies using
// topological sort. Middlewares implementing depgraph.DependencyDeclarer
// declare which other middleware names must precede them. Items without
// dependencies or at the same level maintain their relative position
// (stable sort). Returns an error if a circular dependency is detected.
// See also [Chain.Ordered].
//
// Example:
//
//	middlewares := []Middleware{limiterMiddleware, loggerMiddleware, recoveryMiddleware}
//	ordered, err := middlewares.OrderMiddlewares(middlewares)
//	// ordered: [loggerMiddleware, recoveryMiddleware, limiterMiddleware]
func OrderMiddlewares(middlewares []Middleware) ([]Middleware, error) {
	return orderByDependencies(middlewares, nil)
}

// OrderMiddlewaresWithDedupe sorts middlewares by dependencies (like
// [OrderMiddlewares]) and removes duplicates by name, keeping the first
// occurrence. Returns an error if a circular dependency is detected.
// See also [Chain.OrderedWithDedupe].
//
// Example:
//
//	middlewares := []Middleware{limiter1, logger, limiter2}
//	ordered, err := middlewares.OrderMiddlewaresWithDedupe(middlewares)
//	// ordered: [logger, limiter1] (limiter2 removed as duplicate)
func OrderMiddlewaresWithDedupe(middlewares []Middleware) ([]Middleware, error) {
	ordered, err := orderByDependencies(middlewares, nil)
	if err != nil {
		return nil, err
	}

	// Remove duplicates by name, keeping first occurrence
	seen := make(map[string]struct{}, len(ordered))
	result := make([]Middleware, 0, len(ordered))

	for _, m := range ordered {
		name := m.Name()
		if _, exists := seen[name]; !exists {
			seen[name] = struct{}{}
			result = append(result, m)
		}
	}

	return result, nil
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"log/slog"
	"maps"
	"net/http"

	"github.com/altessa-s/go-atlas/transport/internal/depgraph"
)

// Chain is a chain of HTTP middlewares that provides automatic ordering
// and composition. It accepts any type that implements the [Middleware] interface.
//
// The chain can automatically order middlewares according to their declared
// dependencies using topological sort (via [Chain.Ordered] or
// [Chain.OrderedWithDedupe]), ensuring consistent behavior regardless of
// registration order.
//
// Middlewares implementing depgraph.DependencyDeclarer are automatically
// ordered based on their declared dependencies.
//
// A Chain is NOT safe for concurrent use. All registration (via [Chain.Add],
// [Chain.AddFunc], [Chain.Extend]) must complete before calling [Chain.Then]
// or [Chain.ThenFunc].
//
// Example:
//
//	chain := middlewares.NewChain(
//	    loggerMiddleware,
//	    recoveryMiddleware,
//	    prometheusMiddleware,
//	)
//	handler := chain.Then(myHandler)
//	http.ListenAndServe(":8080", handler)
type Chain struct {
	list      []Middleware
	nameIndex map[string]struct{} // O(1) lookup for middleware names
	logger    *slog.Logger
}

// NewChain creates a new middleware chain from the provided middlewares.
// Middlewares are added in the order provided.
// Use WithAutoOrder() to automatically sort middlewares by their canonical order.
//
// Example:
//
//	chain := middlewares.NewChain(
//	    loggerMiddleware,
//	    authMiddleware,
//	    rateLimiter,
//	)
func NewChain(middlewares ...Middleware) *Chain {
	c := &Chain{
		list:      make([]Middleware, 0, len(middlewares)),
		nameIndex: make(map[string]struct{}, len(middlewares)),
	}
	for _, m := range middlewares {
		if m != nil {
			c.list = append(c.list, m)
			c.nameIndex[m.Name()] = struct{}{}
		}
	}
	return c
}

// Add appends one or more middlewares to the chain.
// Nil middlewares are silently ignored.
// Returns the chain for method chaining.
//
// Example:
//
//	chain := middlewares.NewChain().
//	    Add(loggerMiddleware).
//	    Add(authMiddleware, rateLimiter)
func (c *Chain) Add(middlewares ...Middleware) *Chain {
	for _, m := range middlewares {
		if m != nil {
			c.list = append(c.list, m)
			c.nameIndex[m.Name()] = struct{}{}
		}
	}
	return c
}

// AddFunc adds a middleware from a name and handler function.
// This is a convenience method for adding simple middleware functions.
//
// Example:
//
//	chain.AddFunc("custom", func(next http.Handler) http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        // custom logic
//	        next.ServeHTTP(w, r)
//	    })
//	})
func (c *Chain) AddFunc(name string, handler func(http.Handler) http.Handler) *Chain {
	return c.Add(Func(name, handler))
}

// Len returns the number of middlewares in the chain.
func (c *Chain) Len() int {
	return len(c.list)
}

// Has checks if a middleware with the given name exists in the chain.
func (c *Chain) Has(name string) bool {
	_, exists := c.nameIndex[name]
	return exists
}

// Then applies the middleware chain to the final handler and returns
// the composed http.Handler. Middlewares are applied in the order they
// appear in the chain (first middleware wraps the next, and so on).
//
// Example:
//
//	handler := chain.Then(myHandler)
//	// Equivalent to: middleware1(middleware2(middleware3(myHandler)))
func (c *Chain) Then(final http.Handler) http.Handler {
	if final == nil {
		final = http.DefaultServeMux
	}

	// Apply middlewares in reverse order so the first middleware
	// in the chain is the outermost wrapper
	for i := len(c.list) - 1; i >= 0; i-- {
		final = c.list[i].Handler(final)
	}

	return final
}

// ThenFunc applies the middleware chain to an http.HandlerFunc.
// This is a convenience method equivalent to Then(http.HandlerFunc(fn)).
//
// Example:
//
//	handler := chain.ThenFunc(func(w http.ResponseWriter, r *http.Request) {
//	    w.Write([]byte("Hello, World!"))
//	})
func (c *Chain) ThenFunc(fn func(http.ResponseWriter, *http.Request)) http.Handler {
	return c.Then(http.HandlerFunc(fn))
}

// Ordered returns a new chain with middlewares sorted by canonical order
// using [OrderMiddlewares]. The original chain is not modified.
//
// Example:
//
//	// Even if added out of order, they execute in canonical order
//	chain := middlewares.NewChain(limiter, logger, recovery)
//	ordered, err := chain.Ordered()
//	// ordered executes as: logger -> recovery -> limiter
func (c *Chain) Ordered() (*Chain, error) {
	orderedList, err := OrderMiddlewares(c.list)
	if err != nil {
		return nil, err
	}
	nameIndex := make(map[string]struct{}, len(orderedList))
	for _, m := range orderedList {
		nameIndex[m.Name()] = struct{}{}
	}
	return &Chain{list: orderedList, nameIndex: nameIndex}, nil
}

// OrderedWithDedupe returns a new chain with middlewares sorted by canonical
// order (using [OrderMiddlewaresWithDedupe]) and duplicates removed (keeping
// the first occurrence). The original chain is not modified.
// Returns an error if a circular dependency is detected.
func (c *Chain) OrderedWithDedupe() (*Chain, error) {
	orderedList, err := OrderMiddlewaresWithDedupe(c.list)
	if err != nil {
		return nil, err
	}
	nameIndex := make(map[string]struct{}, len(orderedList))
	for _, m := range orderedList {
		nameIndex[m.Name()] = struct{}{}
	}
	return &Chain{list: orderedList, nameIndex: nameIndex}, nil
}

// Clone returns a copy of the chain.
// Modifications to the copy don't affect the original.
func (c *Chain) Clone() *Chain {
	list := make([]Middleware, len(c.list))
	copy(list, c.list)
	return &Chain{list: list, nameIndex: maps.Clone(c.nameIndex)}
}

// Extend returns a new chain that combines this chain with additional middlewares.
// The original chain is not modified.
//
// Example:
//
//	base := middlewares.NewChain(logger, recovery)
//	extended := base.Extend(auth, limiter)
//	// extended contains: logger, recovery, auth, limiter
func (c *Chain) Extend(middlewares ...Middleware) *Chain {
	newList := make([]Middleware, 0, len(c.list)+len(middlewares))
	newList = append(newList, c.list...)
	nameIndex := maps.Clone(c.nameIndex)
	for _, m := range middlewares {
		if m != nil {
			newList = append(newList, m)
			nameIndex[m.Name()] = struct{}{}
		}
	}
	return &Chain{list: newList, nameIndex: nameIndex}
}

// Middlewares returns a copy of the middleware list.
// This is useful for inspection or debugging.
func (c *Chain) Middlewares() []Middleware {
	result := make([]Middleware, len(c.list))
	copy(result, c.list)
	return result
}

// WithLogger sets a logger for debug output during dependency resolution.
// When set, missing dependencies are logged at Debug level.
func (c *Chain) WithLogger(logger *slog.Logger) *Chain {
	c.logger = logger
	return c
}

// DependencyOrder returns the computed order of middlewares based on dependencies.
// This is useful for debugging and testing the dependency resolution.
// Returns an error if a circular dependency is detected.
func (c *Chain) DependencyOrder() ([]string, error) {
	sorted, err := orderByDependencies(c.list, c.logger)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(sorted))
	for _, m := range sorted {
		if name := m.Name(); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// DependencyGraph returns a map of middleware names to their declared dependencies.
// This is useful for debugging and understanding the dependency structure.
func (c *Chain) DependencyGraph() map[string][]string {
	result := make(map[string][]string)
	for _, m := range c.list {
		name := m.Name()
		if name == "" {
			continue
		}

		if declarer, ok := m.(depgraph.DependencyDeclarer); ok {
			result[name] = declarer.Dependencies()
		} else {
			result[name] = nil
		}
	}
	return result
}

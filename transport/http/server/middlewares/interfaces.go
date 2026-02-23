// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import "net/http"

// Middleware is the interface that all chainable HTTP middlewares must implement.
// It provides a name for identification and dependency-based ordering via
// [Chain], and a handler function for the actual middleware logic.
//
// Implementations that also satisfy [depgraph.DependencyDeclarer] (by
// providing a Dependencies method) are automatically ordered by
// [OrderMiddlewares] and [Chain.Ordered].
//
// The built-in [BaseMiddleware] struct provides a ready-made implementation
// of Name plus common helpers (path filtering, logging, string interning).
type Middleware interface {
	// Name returns the middleware name used for identification and ordering.
	Name() string

	// Handler wraps an http.Handler with middleware functionality.
	// The returned handler should call next.ServeHTTP to continue the chain,
	// or return early to short-circuit the request.
	Handler(next http.Handler) http.Handler
}

// Matcher determines if a middleware should be applied at runtime.
// Unlike [StaticConditionMiddleware] whose condition is fixed at creation
// time, Matcher is evaluated on every request by [ConditionalMiddleware],
// allowing middlewares to be enabled/disabled based on runtime state
// such as feature flags, health checks, or external configuration.
//
// Example:
//
//	type featureFlagMatcher struct {
//	    flags *FeatureFlags
//	    flag  string
//	}
//
//	func (m *featureFlagMatcher) Match() bool {
//	    return m.flags.IsEnabled(m.flag)
//	}
type Matcher interface {
	// Match returns true if the middleware should be applied.
	Match() bool
}

// MatchFunc adapts a function to the Matcher interface.
// This provides a convenient way to create Matchers from simple functions.
//
// Example:
//
//	matcher := middlewares.MatchFunc(func() bool {
//	    return featureFlags.IsEnabled("new-auth")
//	})
//	auth := middlewares.ConditionalMiddleware(matcher, authMiddleware)
type MatchFunc func() bool

// Match implements the Matcher interface by calling the underlying function.
func (f MatchFunc) Match() bool { return f() }

// middlewareFunc is an adapter that allows ordinary functions to be used as Middleware.
type middlewareFunc struct {
	name    string
	handler func(http.Handler) http.Handler
}

func (m *middlewareFunc) Name() string {
	return m.name
}

func (m *middlewareFunc) Handler(next http.Handler) http.Handler {
	return m.handler(next)
}

// Func creates a [Middleware] from a name and handler function.
// This is useful for wrapping simple middleware functions that don't need
// a full struct implementation. For functions that also declare dependencies,
// use [FuncWithDeps].
//
// Example:
//
//	loggingMiddleware := middlewares.Func("logging", func(next http.Handler) http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        log.Printf("Request: %s %s", r.Method, r.URL.Path)
//	        next.ServeHTTP(w, r)
//	    })
//	})
func Func(name string, handler func(http.Handler) http.Handler) Middleware {
	return &middlewareFunc{
		name:    name,
		handler: handler,
	}
}

// middlewareFuncWithDeps extends middlewareFunc with dependency declarations for ordering.
type middlewareFuncWithDeps struct {
	middlewareFunc
	deps []string
}

func (m *middlewareFuncWithDeps) Dependencies() []string { return m.deps }

// FuncWithDeps creates a [Middleware] from a name, dependency list, and handler function.
// The deps parameter lists middleware names that must appear before this one
// in the chain. The returned value also satisfies depgraph.DependencyDeclarer,
// so [OrderMiddlewares] and [Chain.Ordered] sort it correctly.
func FuncWithDeps(name string, deps []string, handler func(http.Handler) http.Handler) Middleware {
	return &middlewareFuncWithDeps{
		middlewareFunc: middlewareFunc{name: name, handler: handler},
		deps:           deps,
	}
}

// conditionalMiddleware wraps a middleware with a runtime condition.
type conditionalMiddleware struct {
	matcher    Matcher
	middleware Middleware
}

func (c *conditionalMiddleware) Name() string {
	return c.middleware.Name()
}

func (c *conditionalMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.matcher.Match() {
			c.middleware.Handler(next).ServeHTTP(w, r)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}

// ConditionalMiddleware wraps a middleware with a runtime condition.
// The middleware is only applied if the Matcher returns true.
// This allows for dynamic enabling/disabling of middlewares based on
// feature flags, health checks, or other runtime state.
//
// Example:
//
//	matcher := middlewares.MatchFunc(func() bool {
//	    return config.AuthEnabled
//	})
//	conditionalAuth := middlewares.ConditionalMiddleware(matcher, authMiddleware)
func ConditionalMiddleware(matcher Matcher, middleware Middleware) Middleware {
	return &conditionalMiddleware{
		matcher:    matcher,
		middleware: middleware,
	}
}

// StaticConditionMiddleware wraps a middleware with a static boolean condition.
// Unlike ConditionalMiddleware, this condition is evaluated once at creation time.
// Use this when the condition won't change during the application's lifetime.
//
// Example:
//
//	authMiddleware := middlewares.StaticConditionMiddleware(config.AuthEnabled, authMiddleware)
func StaticConditionMiddleware(condition bool, middleware Middleware) Middleware {
	if !condition {
		return &noopMiddleware{name: middleware.Name()}
	}
	return middleware
}

// noopMiddleware is a middleware that does nothing.
type noopMiddleware struct {
	name string
}

func (n *noopMiddleware) Name() string {
	return n.name
}

func (n *noopMiddleware) Handler(next http.Handler) http.Handler {
	return next
}

// Noop returns a no-op middleware with the given name.
// It simply passes requests through to the next handler unchanged.
func Noop(name string) Middleware {
	return &noopMiddleware{name: name}
}

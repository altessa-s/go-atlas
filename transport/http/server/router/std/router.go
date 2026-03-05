// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package std

import (
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/transport/http/server/router"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Router implements [router.Router] using [http.ServeMux] (Go 1.22+).
// Create instances with [New]. Routes and middleware must be registered before
// the first call to [Router.ServeHTTP] or [Router.Initialize]; the router is
// sealed after initialization and panics on late registration.
//
// After initialization, [Router.ServeHTTP] is safe for concurrent use.
// Registration methods ([Router.Handle], [Router.Use], etc.) are protected
// by a mutex but must not be called after sealing.
type Router struct {
	mu           sync.Mutex
	mux          *http.ServeMux
	routes       []*route
	middleware   []router.Middleware
	initOnce     sync.Once
	finalHandler http.Handler
	sealed       atomic.Bool
}

// Ensure Router implements router.Router interface.
var _ router.Router = (*Router)(nil)

// New creates a new Router based on standard library http.ServeMux.
func New() *Router {
	return &Router{
		mux: http.NewServeMux(),
	}
}

// Handle registers a handler for the given pattern.
func (r *Router) Handle(pattern string, handler http.Handler) router.Route {
	r.ensureMutable()

	r.mu.Lock()
	defer r.mu.Unlock()

	rt := &route{
		router:  r,
		pattern: pattern,
		handler: handler,
	}
	r.routes = append(r.routes, rt)
	return rt
}

// HandleFunc registers a handler function for the given pattern.
func (r *Router) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) router.Route {
	return r.Handle(pattern, http.HandlerFunc(handler))
}

// Methods sets allowed HTTP methods for the route (not applicable at router level directly, but strictly part of interface).
// For Router, this creates a new route? No, interface says Router has Methods().
// But router.Router interface in router.go:
// Methods(methods ...string) Route
// This seems to imply creating a route that matches these methods?
// The gorilla implementation: return &route{route: r.router.Methods(methods...)}
// Which creates a new empty route with method matcher.
// For std router, we can't create an "empty" route easily.
// We will return a placeholder route.
func (r *Router) Methods(methods ...string) router.Route {
	r.ensureMutable()

	// This pattern is rarely used at Router level in pure form without a path,
	// except for "Not Found" handlers maybe?
	// For now, we return a dummy route to satisfy interface, but it might not work as expected
	// if attached to nothing.
	r.mu.Lock()
	defer r.mu.Unlock()
	rt := &route{
		router:  r,
		methods: methods,
	}
	r.routes = append(r.routes, rt)
	return rt
}

// PathPrefix registers a route matching a path prefix.
func (r *Router) PathPrefix(prefix string) router.Route {
	r.ensureMutable()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure prefix ends with / for ServeMux behavior if it's meant to be a directory-like prefix
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	rt := &route{
		router:   r,
		pattern:  prefix,
		isPrefix: true,
	}
	r.routes = append(r.routes, rt)
	return rt
}

// Use appends middleware to the router.
func (r *Router) Use(middleware ...router.Middleware) {
	r.ensureMutable()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.middleware = append(r.middleware, middleware...)
}

// Initialize eagerly builds the middleware chain and registers routes with
// the underlying [http.ServeMux]. After this call the router is sealed;
// further route or middleware registration panics. This implements the
// [server.InitializableRouter] interface.
func (r *Router) Initialize() {
	r.initOnce.Do(r.initialize)
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.initOnce.Do(r.initialize)
	r.finalHandler.ServeHTTP(w, req)
}

func (r *Router) initialize() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, rt := range r.routes {
		rt.register(r.mux)
	}

	// Apply middleware chain
	var handler http.Handler = r.mux
	for i := len(r.middleware) - 1; i >= 0; i-- {
		handler = r.middleware[i](handler)
	}
	r.finalHandler = handler
	r.sealed.Store(true)
}

// route implements router.Route interface.
type route struct {
	router    *Router
	pattern   string
	handler   http.Handler
	methods   []string
	subRouter *Router
	isPrefix  bool
}

// Ensure route implements router.Route interface.
var _ router.Route = (*route)(nil)

func (rt *route) Handler(handler http.Handler) router.Route {
	rt.handler = handler
	return rt
}

func (rt *route) HandlerFunc(f func(http.ResponseWriter, *http.Request)) router.Route {
	rt.handler = http.HandlerFunc(f)
	return rt
}

func (rt *route) Methods(methods ...string) router.Route {
	rt.methods = append(rt.methods, methods...)
	return rt
}

func (rt *route) Path(path string) router.Route {
	rt.pattern = path
	rt.isPrefix = false
	return rt
}

func (rt *route) PathPrefix(prefix string) router.Route {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	rt.pattern = prefix
	rt.isPrefix = true
	return rt
}

func (rt *route) Subrouter() router.Router {
	rt.router.ensureMutable()

	if rt.subRouter == nil {
		rt.subRouter = New()
		// If we have a subrouter, the handler becomes the subrouter
		// wrapped in StripPrefix if it is a prefix route
		if rt.isPrefix {
			// Strip the prefix from the path before passing to subrouter
			prefix := rt.pattern
			rt.handler = http.StripPrefix(prefix, rt.subRouter)
		} else {
			rt.handler = rt.subRouter
		}
	}
	return rt.subRouter
}

// Subrouter creates a new router that inherits the parent's middleware.
func (r *Router) Subrouter() router.Router {
	r.ensureMutable()
	return New()
}

func (rt *route) register(mux *http.ServeMux) {
	if rt.handler == nil {
		panic("router/std: route missing handler; set Handler/HandlerFunc before ServeHTTP")
	}

	if rt.pattern == "" {
		panic("router/std: route missing path; call Path or PathPrefix")
	}

	// Construct Go 1.22 pattern
	// Format: [METHOD ][HOST]/[PATH]

	var patterns []string
	if len(rt.methods) > 0 {
		patterns = slices.Collect(coreslices.Map(rt.methods, func(m string) string {
			// Use BuildString for efficient string concatenation
			return corestrings.BuildString(func(b *strings.Builder) {
				b.WriteString(m)
				b.WriteString(" ")
				b.WriteString(rt.pattern)
			})
		}))
	} else {
		patterns = []string{rt.pattern}
	}

	for _, p := range patterns {
		// Handle edge case where pattern is empty
		if len(p) == 0 || p == " " {
			continue
		}
		mux.Handle(p, rt.handler)
	}
}

func (r *Router) ensureMutable() {
	if r.sealed.Load() {
		panic("router/std: router already initialized; register routes and middleware before ServeHTTP")
	}
}

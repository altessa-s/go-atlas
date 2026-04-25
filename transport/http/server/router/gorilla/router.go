// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gorilla

import (
	"net/http"
	"slices"

	"github.com/gorilla/mux"

	"github.com/altessa-s/go-atlas/transport/http/server/router"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// Router wraps gorilla/mux.Router to implement router.Router interface.
type Router struct {
	router *mux.Router
}

// Ensure Router implements router.Router interface.
var _ router.Router = (*Router)(nil)

// New creates a new Router.
func New() *Router {
	return &Router{router: mux.NewRouter()}
}

// Handle registers a handler for the given pattern.
func (r *Router) Handle(pattern string, handler http.Handler) router.Route {
	return &route{route: r.router.Handle(pattern, handler)}
}

// HandleFunc registers a handler function for the given pattern.
func (r *Router) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) router.Route {
	return &route{route: r.router.HandleFunc(pattern, handler)}
}

// Methods sets allowed HTTP methods for the route.
func (r *Router) Methods(methods ...string) router.Route {
	return &route{route: r.router.Methods(methods...)}
}

// PathPrefix registers a route matching a path prefix.
func (r *Router) PathPrefix(prefix string) router.Route {
	return &route{route: r.router.PathPrefix(prefix)}
}

// Use appends middleware to the router.
func (r *Router) Use(middleware ...router.Middleware) {
	muxMiddleware := slices.Collect(coreslices.Map(middleware, func(m router.Middleware) mux.MiddlewareFunc {
		return mux.MiddlewareFunc(m)
	}))
	r.router.Use(muxMiddleware...)
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.router.ServeHTTP(w, req)
}

// Subrouter creates a new router that inherits the parent's middleware.
func (r *Router) Subrouter() router.Router {
	return &Router{router: r.router.NewRoute().Subrouter()}
}

// Underlying returns the underlying mux.Router for advanced use cases.
func (r *Router) Underlying() *mux.Router {
	return r.router
}

// route wraps gorilla/mux.Route to implement router.Route interface.
type route struct {
	route *mux.Route
}

// Ensure route implements router.Route interface.
var _ router.Route = (*route)(nil)

// Handler sets the handler for the route.
func (r *route) Handler(handler http.Handler) router.Route {
	r.route.Handler(handler)
	return r
}

// HandlerFunc sets the handler function for the route.
func (r *route) HandlerFunc(f func(http.ResponseWriter, *http.Request)) router.Route {
	r.route.HandlerFunc(f)
	return r
}

// Methods sets allowed HTTP methods for the route.
func (r *route) Methods(methods ...string) router.Route {
	r.route.Methods(methods...)
	return r
}

// Path sets the path for the route.
func (r *route) Path(path string) router.Route {
	r.route.Path(path)
	return r
}

// PathPrefix sets the path prefix for the route.
func (r *route) PathPrefix(prefix string) router.Route {
	r.route.PathPrefix(prefix)
	return r
}

// Subrouter creates a subrouter for this route.
func (r *route) Subrouter() router.Router {
	return &Router{router: r.route.Subrouter()}
}

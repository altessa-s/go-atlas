// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package router

import (
	"net/http"
)

// Middleware is a function that wraps an HTTP handler.
// It provides a way to intercept and modify requests/responses.
type Middleware func(http.Handler) http.Handler

// Router abstracts HTTP routing functionality, hiding the underlying router
// implementation details. Available implementations include gorilla/mux
// (package gorilla) and Go 1.22+ [http.ServeMux] (package std).
type Router interface {
	// Handle registers a handler for the given pattern.
	Handle(pattern string, handler http.Handler) Route

	// HandleFunc registers a handler function for the given pattern.
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) Route

	// Methods sets allowed HTTP methods for the route.
	Methods(methods ...string) Route

	// PathPrefix registers a route matching a path prefix.
	PathPrefix(prefix string) Route

	// Use appends middleware to the router.
	Use(middleware ...Middleware)

	// ServeHTTP implements http.Handler.
	ServeHTTP(http.ResponseWriter, *http.Request)

	// Subrouter creates a new router that inherits the parent's middleware.
	Subrouter() Router
}

// Route represents a registered route.
type Route interface {
	// Handler sets the handler for the route.
	Handler(handler http.Handler) Route

	// HandlerFunc sets the handler function for the route.
	HandlerFunc(f func(http.ResponseWriter, *http.Request)) Route

	// Methods sets allowed HTTP methods for the route.
	Methods(methods ...string) Route

	// Path sets the path for the route.
	Path(path string) Route

	// PathPrefix sets the path prefix for the route.
	PathPrefix(prefix string) Route

	// Subrouter creates a subrouter for this route.
	Subrouter() Router
}

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

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

// Router implements [router.Router] on one [http.ServeMux] with Go 1.22+
// patterns. Create instances with [New]. A subrouter shares its parent's
// mux: its routes are registered under its full path prefix, and the
// middleware added to it with [Router.Use] wraps only those routes (and its
// own subrouters' routes), while the root router's middleware wraps every
// request, unmatched ones included.
//
// Routes and middleware must be registered before the first call to
// [Router.ServeHTTP] or [Router.Initialize] on any router of the tree; the
// tree is sealed after initialization and panics on late registration. After
// initialization [Router.ServeHTTP] is safe for concurrent use and serves the
// whole tree from any of its routers.
type Router struct {
	tree       *tree
	parent     *Router
	prefix     string
	middleware []router.Middleware
	// routes are the routes registered on this router, kept for
	// white-box tests; the tree holds every route of every router.
	routes []*route
}

// tree is the state a router shares with all its subrouters.
type tree struct {
	mu           sync.Mutex
	mux          *http.ServeMux
	root         *Router
	routes       []*route
	initOnce     sync.Once
	finalHandler http.Handler
	sealed       atomic.Bool
}

// Ensure Router implements router.Router interface.
var _ router.Router = (*Router)(nil)

// New creates a root Router over a new [http.ServeMux].
func New() *Router {
	t := &tree{mux: http.NewServeMux()}
	t.root = &Router{tree: t}
	return t.root
}

// Handle registers a handler for the given pattern, relative to the router's
// prefix.
func (r *Router) Handle(pattern string, handler http.Handler) router.Route {
	return r.add(&route{pattern: pattern, handler: handler})
}

// HandleFunc registers a handler function for the given pattern.
func (r *Router) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) router.Route {
	return r.Handle(pattern, http.HandlerFunc(handler))
}

// Methods starts a route restricted to methods; set its path with
// [router.Route.Path] or [router.Route.PathPrefix] and its handler before
// the router is initialized.
func (r *Router) Methods(methods ...string) router.Route {
	return r.add(&route{methods: slices.Clone(methods)})
}

// PathPrefix starts a route matching prefix and every path below it. Set its
// handler, or call Subrouter on it to register routes under the prefix.
func (r *Router) PathPrefix(prefix string) router.Route {
	return r.add(&route{pattern: prefix, isPrefix: true})
}

// Use appends middleware to the router. On the root router it wraps every
// request; on a subrouter it wraps the subrouter's routes.
func (r *Router) Use(middleware ...router.Middleware) {
	r.ensureMutable()
	r.tree.mu.Lock()
	defer r.tree.mu.Unlock()
	r.middleware = append(r.middleware, middleware...)
}

// Subrouter creates a router with the same prefix whose middleware applies to
// its own routes only.
func (r *Router) Subrouter() router.Router {
	r.ensureMutable()
	return &Router{tree: r.tree, parent: r, prefix: r.prefix}
}

// Initialize eagerly registers every route of the tree with the underlying
// [http.ServeMux] and builds the root middleware chain. After this call the
// tree is sealed; further registration panics. It implements
// server.InitializableRouter.
func (r *Router) Initialize() {
	r.tree.initOnce.Do(r.tree.initialize)
}

// ServeHTTP implements http.Handler for the whole tree.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.tree.initOnce.Do(r.tree.initialize)
	r.tree.finalHandler.ServeHTTP(w, req)
}

func (r *Router) add(rt *route) *route {
	r.ensureMutable()
	rt.router = r
	r.tree.mu.Lock()
	defer r.tree.mu.Unlock()
	r.routes = append(r.routes, rt)
	r.tree.routes = append(r.tree.routes, rt)
	return rt
}

func (r *Router) ensureMutable() {
	if r.tree.sealed.Load() {
		panic("router/std: router already initialized; register routes and middleware before ServeHTTP")
	}
}

// chain wraps h in the middleware of r and its ancestors up to, but not
// including, the root, whose middleware wraps the whole mux instead. Outer
// routers' middleware runs first.
func (r *Router) chain(h http.Handler) http.Handler {
	for cur := r; cur != nil && cur.parent != nil; cur = cur.parent {
		for _, mw := range slices.Backward(cur.middleware) {
			h = mw(h)
		}
	}
	return h
}

func (t *tree) initialize() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, rt := range t.routes {
		rt.register(t.mux)
	}
	var handler http.Handler = t.mux
	for _, mw := range slices.Backward(t.root.middleware) {
		handler = mw(handler)
	}
	t.finalHandler = handler
	t.sealed.Store(true)
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
	rt.pattern = prefix
	rt.isPrefix = true
	return rt
}

// Subrouter returns a router whose routes are registered under this route's
// full path; the route itself then registers nothing. Repeated calls return
// the same subrouter.
func (rt *route) Subrouter() router.Router {
	rt.router.ensureMutable()
	if rt.subRouter == nil {
		rt.subRouter = &Router{tree: rt.router.tree, parent: rt.router, prefix: joinPath(rt.router.prefix, rt.pattern)}
	}
	return rt.subRouter
}

// register adds the route to mux under its full path, one pattern per method.
func (rt *route) register(mux *http.ServeMux) {
	if rt.subRouter != nil {
		return
	}
	if nilcheck.IsNil(rt.handler) {
		panic("router/std: route missing handler; set Handler/HandlerFunc before ServeHTTP")
	}
	if rt.pattern == "" {
		panic("router/std: route missing path; call Path or PathPrefix")
	}
	methods := rt.methods
	pattern := rt.pattern
	// A Go pattern may carry its method ("GET /users/{id}"); the prefix goes
	// before the path, not before the method.
	if method, rest, ok := strings.Cut(pattern, " "); ok && !strings.HasPrefix(method, "/") {
		methods = append(slices.Clone(methods), method)
		pattern = strings.TrimLeft(rest, " ")
	}
	path := joinPath(rt.router.prefix, pattern)
	if rt.isPrefix && !strings.HasSuffix(path, "/") {
		path += "/"
	}
	handler := rt.router.chain(rt.handler)
	if len(methods) == 0 {
		mux.Handle(path, handler)
		return
	}
	for _, m := range methods {
		mux.Handle(m+" "+path, handler)
	}
}

// joinPath appends pattern to prefix with exactly one slash between them; a
// root pattern ("/") under a prefix keeps the prefix's trailing slash, so it
// matches the prefix's subtree as ServeMux does.
func joinPath(prefix, pattern string) string {
	if prefix == "" {
		return pattern
	}
	return strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(pattern, "/")
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"context"
	"errors"
	"iter"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/transport/http/server/responder"
	"github.com/altessa-s/go-atlas/transport/http/server/router"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

// Re-export router types for convenience.
type (
	// Router abstracts HTTP routing functionality.
	Router = router.Router
	// Route represents a registered route.
	Route = router.Route
	// Middleware is a function that wraps an HTTP handler.
	Middleware = router.Middleware
)

// ResponseWriter is the contract for request/response helpers used by handlers.
// The default implementation is [writer.Writer]. Implementations must be able
// to build a per-request [writer.ReadWriter].
type ResponseWriter interface {
	// Write writes a response with content negotiation.
	Write(w http.ResponseWriter, r *http.Request, data any) error

	// Read reads and decodes request body into out.
	Read(r *http.Request, out any) error

	// WriteError writes an error response.
	WriteError(w http.ResponseWriter, r *http.Request, err error, statusCode ...int) error

	// NewReadWriter creates a ReadWriter for handling a request.
	NewReadWriter(w http.ResponseWriter, r *http.Request) writer.ReadWriter
}

// InitializableRouter is an optional interface that Router implementations
// can implement to support eager initialization before the first request.
// This allows building middleware chains and registering routes before Start()
// instead of lazily on the first request.
type InitializableRouter interface {
	// Initialize forces initialization of the router (routes and middleware chain).
	// This should be called before Start() to avoid lazy initialization overhead.
	Initialize()
}

// Ensure Server implements RouteRegistrar interface.
var _ RouteRegistrar = (*Server)(nil)

// Server is an HTTP server with graceful shutdown and middleware support.
// All route and middleware registration must happen before [Server.Start].
// The zero value is not usable; create instances with [New].
type Server struct {
	*baseserver.BaseServer

	stopCh         chan struct{}
	http           *http.Server
	router         Router
	readTimeout    time.Duration
	writeTimeout   time.Duration
	idleTimeout    time.Duration
	maxHeaderBytes int
	writer         ResponseWriter
	middleware     []Middleware
	handlers       []Handler
}

// New creates a new HTTP [Server]. A [Router] is required (via [WithRouter]);
// if no [ResponseWriter] is provided, a default [writer.Writer] is used.
// Returns an error if the router is nil.
func New(opts ...Option) (*Server, error) {
	cfg := newOptions(opts...)

	if cfg.router == nil {
		return nil, errors.New("router is required")
	}

	if cfg.writer == nil {
		cfg.writer = writer.New()
	}

	srv := &Server{
		BaseServer:     baseserver.NewBaseServer(cfg.baseOpts...),
		stopCh:         make(chan struct{}),
		router:         cfg.router,
		readTimeout:    cfg.readTimeout,
		writeTimeout:   cfg.writeTimeout,
		idleTimeout:    cfg.idleTimeout,
		maxHeaderBytes: cfg.maxHeaderBytes,
		writer:         cfg.writer,
	}

	return srv, nil
}

// RegisterHandlers registers HTTP handlers. Call before [Server.Start].
// Nil handlers are silently filtered out.
func (s *Server) RegisterHandlers(handlers ...Handler) {
	s.handlers = append(s.handlers, slices.Collect(coreslices.Filter(handlers, func(h Handler) bool {
		return nilcheck.IsNotNil(h)
	}))...)
}

// RegisterMiddleware registers HTTP middleware. Call before [Server.Start].
// Nil middleware entries are silently filtered out.
func (s *Server) RegisterMiddleware(middleware ...Middleware) {
	// Filter out nil middleware and append
	s.middleware = append(s.middleware, slices.Collect(coreslices.Filter(middleware, func(m Middleware) bool {
		return nilcheck.IsNotNil(m)
	}))...)
}

// Handle registers a HandlerFunc for the given pattern.
// Returns Route for further configuration (Methods, PathPrefix, etc.)
//
// Example:
//
//	server.Handle("/users", createUserHandler).Methods("POST")
//	server.Handle("/users/{id}", getUserHandler).Methods("GET")
//
// Note: Handle() should be called before Start(). Registering routes after Start()
// may work but is not recommended and may cause race conditions.
func (s *Server) Handle(pattern string, handler HandlerFunc) Route {
	// Warn if the server is already started (routes should be registered before Start())
	if s.IsStarted() && s.Logger() != nil {
		s.Logger().Warn("registering route after server start, this may cause race conditions",
			"pattern", pattern)
	}

	return s.router.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		rw := s.writer.NewReadWriter(w, r)
		defer rw.Release()
		handler(rw)
	})
}

// IsStarted returns true if the server is running.
func (s *Server) IsStarted() bool {
	return s.BaseServer.IsStarted()
}

// IsStopped returns true if the server is stopped.
func (s *Server) IsStopped() bool {
	return s.IsShutdown()
}

// errorInterceptorMiddleware returns middleware that wraps ResponseWriter
// to intercept error responses and convert them to structured responses.
func (s *Server) errorInterceptorMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Inject the writer into context for explicit WriteError usage
			ctx := responder.NewContext(r.Context(), s.writer)
			r = r.WithContext(ctx)

			// Wrap ResponseWriter with an error interceptor
			interceptor := responder.NewErrorInterceptor(w, r, s.writer)
			defer interceptor.Flush()

			next.ServeHTTP(interceptor, r)
		})
	}
}

// Start starts the HTTP server. It applies middleware, registers handlers,
// optionally initializes the router, and begins listening. If TLS is
// configured, the server uses ServeTLS. Returns an error if the writer is
// nil or the underlying listener fails to bind.
func (s *Server) Start() (err error) {
	// Validate required components
	if s.writer == nil {
		return errors.New("writer is required, use WithWriter option")
	}

	// Apply error interceptor middleware first (must run before other middleware)
	// This wraps ResponseWriter to intercept http.Error calls and convert them
	// to structured responses using writer + builder pattern
	s.router.Use(s.errorInterceptorMiddleware())

	// Apply middleware to router
	if len(s.middleware) > 0 {
		s.router.Use(s.middleware...)
	}

	// Register handlers
	for _, h := range s.handlers {
		h.Register(s, s.stopCh)
	}

	// Initialize router if it supports eager initialization
	// This builds middleware chain and registers routes before first request
	if initRouter, ok := s.router.(InitializableRouter); ok {
		initRouter.Initialize()
	}

	// Configure HTTP server
	s.http = &http.Server{
		Handler:        s.router,
		TLSConfig:      s.TLSConfig(),
		ReadTimeout:    s.readTimeout,
		WriteTimeout:   s.writeTimeout,
		IdleTimeout:    s.idleTimeout,
		MaxHeaderBytes: s.maxHeaderBytes,
	}

	// Use BaseServer.Start with HTTP-specific callback
	return s.BaseServer.Start("http", func(ln net.Listener, errCh chan<- error) {
		go func() {
			defer panics.Handle(context.Background())

			var serveErr error
			if s.TLSConfig() != nil {
				// TLS config is set in http.Server, so we just need to call ServeTLS
				// with empty cert and key files since TLSConfig is already configured
				serveErr = s.http.ServeTLS(ln, "", "")
			} else {
				serveErr = s.http.Serve(ln)
			}

			if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				// Non-blocking send to avoid panic if channel is no longer being read
				select {
				case errCh <- serveErr:
				default:
				}
			}
		}()
	})
}

// Handlers returns an iterator over registered handlers.
func (s *Server) Handlers() iter.Seq[Handler] {
	return slices.Values(s.handlers)
}

// Middleware returns an iterator over registered middleware.
func (s *Server) Middleware() iter.Seq[Middleware] {
	return slices.Values(s.middleware)
}

// Shutdown gracefully stops the HTTP server. It closes the stop channel
// (signaling handlers), closes the listener to reject new connections, and
// drains in-flight requests. The provided context controls the shutdown
// deadline; if it expires, the server returns the context's error.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.GracefulShutdown(ctx, func(ctx context.Context) error {
		close(s.stopCh)

		// Close listener first to prevent new connections
		if s.Listener() != nil {
			_ = s.Listener().Close()
		}

		return s.http.Shutdown(ctx)
	})
}

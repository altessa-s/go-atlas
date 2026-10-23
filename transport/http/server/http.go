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
	"sync"
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

	routerOnce        sync.Once
	stopCh            chan struct{}
	http              *http.Server
	router            Router
	readTimeout       time.Duration
	readHeaderTimeout time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	maxHeaderBytes    int
	writer            ResponseWriter
	middleware        []Middleware
	handlers          []Handler
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
		BaseServer:        baseserver.NewBaseServer(cfg.baseOpts...),
		stopCh:            make(chan struct{}),
		router:            cfg.router,
		readTimeout:       cfg.readTimeout,
		readHeaderTimeout: cfg.readHeaderTimeout,
		writeTimeout:      cfg.writeTimeout,
		idleTimeout:       cfg.idleTimeout,
		maxHeaderBytes:    cfg.maxHeaderBytes,
		writer:            cfg.writer,
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
			defer interceptor.Finish()

			next.ServeHTTP(interceptor, r)
		})
	}
}

// Start starts the HTTP server. It applies middleware, registers handlers,
// optionally initializes the router, and begins listening. If TLS is
// configured, the server uses ServeTLS. Returns an error if the writer is
// nil or the underlying listener fails to bind.
func (s *Server) Start() (err error) {
	if err := s.CanStart(); err != nil {
		return err
	}
	// Validate required components
	if s.writer == nil {
		return errors.New("writer is required, use WithWriter option")
	}

	s.routerOnce.Do(func() {
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
	})

	var srv *http.Server
	startErr := s.BaseServer.Start("http", func(ln net.Listener, errCh chan<- error) {
		// Configure HTTP server.
		//
		// ReadHeaderTimeout caps the header-read phase independently of the
		// full ReadTimeout. Without it, the per-request budget is the full
		// ReadTimeout (15s by default; up to 300s under config-level
		// overrides), which a Slowloris-style client can use to pin a
		// goroutine per connection by trickling header bytes. ReadHeaderTimeout
		// (default DefaultReadHeaderTimeout) bounds the header phase to
		// seconds while keeping the body-read budget unchanged.
		local := &http.Server{
			Handler:           s.router,
			TLSConfig:         s.TLSConfig(),
			ReadTimeout:       s.readTimeout,
			ReadHeaderTimeout: s.readHeaderTimeout,
			WriteTimeout:      s.writeTimeout,
			IdleTimeout:       s.idleTimeout,
			MaxHeaderBytes:    s.maxHeaderBytes,
		}
		srv = local
		// Publish under the lifecycle lock before shutdown can claim cleanup.
		s.http = local

		go func() {
			defer panics.Handle(context.Background())

			var serveErr error
			if s.TLSConfig() != nil {
				// TLS config is set in http.Server, so we just need to call ServeTLS
				// with empty cert and key files since TLSConfig is already configured
				serveErr = local.ServeTLS(ln, "", "")
			} else {
				serveErr = local.Serve(ln)
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
	if startErr != nil && srv != nil {
		_ = srv.Close()
	}
	return startErr
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
// (signaling handlers), then delegates to [http.Server.Shutdown] which
// closes the listener, rejects new connections, and drains in-flight
// requests. The provided context controls the shutdown deadline; if it
// is canceled or expires, active connections are force-closed and the context
// error is returned. Force-close may finish after Shutdown returns.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.GracefulShutdown(ctx, func(ctx context.Context) error {
		// The lifecycle claim synchronizes this read with Start and prevents replacement.
		srv := s.http
		close(s.stopCh)

		// http.Server.Shutdown sets the shutdown flag, closes tracked
		// listeners (preventing new connections), and waits for active
		// requests to complete. No manual Listener().Close() is needed —
		// doing so before Shutdown causes a double-close error because
		// http.Serve wraps the listener in a onceCloseListener that
		// records the "use of closed network connection" error and
		// propagates it through Shutdown's return value.
		err := srv.Shutdown(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_ = srv.Close()
		}
		return err
	})
}

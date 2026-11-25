// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"slices"

	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/internal/transport/timeouts"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/lifecycle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	baseserver "github.com/altessa-s/go-atlas/internal/transport/server"
)

// Lifecycle errors returned by [Server.Start].
var (
	// ErrServerAlreadyStarted indicates that the server is already running.
	ErrServerAlreadyStarted = baseserver.ErrServerAlreadyStarted
	// ErrServerClosed indicates that the server cannot be restarted after Done closes.
	ErrServerClosed = baseserver.ErrServerClosed
)

// GRPCServer is the contract for a managed gRPC server, including lifecycle and registration.
// All methods are safe for concurrent use after the server has started.
type GRPCServer interface {
	// Start begins serving and verifies startup.
	Start() error
	// Shutdown drains active calls within the available deadline.
	Shutdown(ctx context.Context) error
	// Done closes after shutdown or a post-start serving failure.
	Done() <-chan struct{}
	// Err reports the first post-start serving failure.
	Err() error

	// RegisterHandlers registers gRPC service handlers with the server.
	// Must be called before Start().
	RegisterHandlers(handlers ...Handler)

	// RegisterInterceptors registers one or more gRPC server interceptors.
	// Must be called before Start().
	// Returns an error if a circular dependency is detected.
	RegisterInterceptors(lst ...interceptors.ServerInterceptor) error
}

// Ensure Server implements GRPCServer interface.
var _ GRPCServer = (*Server)(nil)

// Server wraps a [grpc.Server] with lifecycle management, TLS support,
// reflection, and interceptor chaining.
//
// Typical sequence: [New] -> RegisterHandlers -> RegisterInterceptors -> [Server.Start] -> [Server.Shutdown].
// All exported methods are safe for concurrent use. A stopped Server cannot be restarted.
type Server struct {
	base *baseserver.BaseServer

	stopCh      chan struct{}       // coordination channel for shutdown
	grpcOptions []grpc.ServerOption // additional gRPC server options

	// lifecycle is a one-slot semaphore serializing Start with the shutdown
	// claim, so a running server can never be replaced or leaked. A channel
	// instead of a mutex lets Shutdown give up when its context expires.
	lifecycle chan struct{}
	grpc      *grpc.Server // underlying gRPC server; guarded by lifecycle
	stopped   bool         // set once Shutdown claims the server; guarded by lifecycle

	// interceptors contains registered server interceptors
	interceptors []any

	// handlers contain registered service handlers
	handlers []Handler

	reflection bool // enables gRPC reflection service
}

// New creates a new gRPC server with the provided options.
// The server is created in a stopped state and must be started with Start().
//
// Default configuration:
//   - Uses a discard logger (no logging output)
//   - No TLS configuration (insecure connections)
//   - Reflection disabled
//   - Default address :0
//   - No custom gRPC options
//
// Returns a configured Server instance ready for service registration and startup.
// The error return is reserved for future use and currently always returns nil.
func New(opts ...Option) (*Server, error) {
	// Create config with defaults
	cfg := newOptions(opts...)

	if !cfg.lifecycleTimeoutsSet {
		cfg.lifecycleTimeouts.StartupVerification = max(cfg.startupVerification, 0)
	}

	// Create server
	srv := &Server{
		base: baseserver.NewBaseServer(
			baseserver.WithAddress(cfg.address),
			baseserver.WithName(cfg.name),
			baseserver.WithLogger(cfg.logger),
			baseserver.WithListener(cfg.listener),
			baseserver.WithTLSConfig(cfg.tlsConfig),
			baseserver.WithTimeouts(timeouts.Lifecycle(cfg.lifecycleTimeouts)),
		),
		stopCh:      make(chan struct{}),
		lifecycle:   make(chan struct{}, 1),
		grpcOptions: cfg.grpcOpts,
		reflection:  cfg.reflection,
	}

	// Add TLS credentials if configured
	srv.grpcOptions = coreslices.AppendIfFunc(srv.grpcOptions, srv.TLSConfig() != nil, func() []grpc.ServerOption {
		return []grpc.ServerOption{grpc.Creds(credentials.NewTLS(srv.TLSConfig()))}
	})

	return srv, nil
}

// RegisterHandlers stores handlers for later registration with the underlying
// [grpc.Server]. Nil handlers are silently filtered. Must be called before
// [Server.Start]; not safe for concurrent use during setup.
func (s *Server) RegisterHandlers(handlers ...Handler) {
	s.handlers = slices.AppendSeq(s.handlers, coreslices.Filter(handlers, func(h Handler) bool {
		return nilcheck.IsNotNil(h)
	}))
}

// RegisterInterceptors merges the given interceptors with any previously
// registered ones. Nil interceptors are silently filtered. Ordering,
// deduplication, and cycle detection are deferred to [interceptors.Chain]
// at [Server.Start] time. Must be called before Start; not safe for
// concurrent use during setup.
func (s *Server) RegisterInterceptors(lst ...interceptors.ServerInterceptor) error {
	// Filter out nil interceptors
	filtered := slices.Collect(coreslices.Filter(lst, func(i interceptors.ServerInterceptor) bool {
		return nilcheck.IsNotNil(i)
	}))

	s.interceptors = append(s.interceptors, coreslices.ToAny(filtered)...)
	return nil
}

// Done closes after shutdown or a post-start serving failure.
func (s *Server) Done() <-chan struct{} { return s.base.Done() }

// Err reports the first post-start serving failure. It is nil after normal shutdown.
func (s *Server) Err() error { return s.base.Err() }

// Address returns the bound address, or the configured address before binding.
func (s *Server) Address() string { return s.base.Address() }

// Listener returns the supplied or bound listener, if any.
func (s *Server) Listener() net.Listener { return s.base.Listener() }

// TLSConfig returns the configured TLS settings.
func (s *Server) TLSConfig() *tls.Config { return s.base.TLSConfig() }

// LifecycleTimeouts returns the effective startup and shutdown durations.
func (s *Server) LifecycleTimeouts() lifecycle.Timeouts { return lifecycle.Timeouts(s.base.Timeouts()) }

// IsStarted returns true if the server has been started and is currently running.
func (s *Server) IsStarted() bool {
	return s.base.IsStarted()
}

// IsStopped returns true if the server has been stopped.
func (s *Server) IsStopped() bool {
	return s.base.IsShutdown()
}

// Start starts the gRPC server and begins accepting connections.
// It performs the following operations in sequence:
//  1. Validates server is not already started
//  2. Registers reflection service if enabled
//  3. Creates TCP listener if none provided ([WithListener])
//  4. Starts serving in a background goroutine
//  5. Waits for startup verification ([WithStartupVerification], default
//     [DefaultStartupVerification]; zero skips the wait)
//
// Returns ErrServerAlreadyStarted if called multiple times and
// ErrServerClosed once the server has been shut down; the running server is
// left untouched in both cases. Retry is meaningful for listen/bind or pre-Serve
// errors; a WithListener listener may be consumed by a failed Serve.
// Returns network errors if listener creation or binding fails.
// This method is non-blocking after successful startup verification.
func (s *Server) Start() (err error) {
	s.lifecycle <- struct{}{}
	defer func() { <-s.lifecycle }()

	if err := s.base.CanStart(); err != nil {
		return err
	}
	if s.stopped {
		return baseserver.ErrServerClosed
	}
	if s.grpc != nil {
		return baseserver.ErrServerAlreadyStarted
	}

	// Prepare gRPC server options before starting
	grpcOptions := make([]grpc.ServerOption, 0, len(s.interceptors)+len(s.grpcOptions))
	grpcOptions = append(grpcOptions, s.grpcOptions...)

	if len(s.interceptors) > 0 {
		serverOpts, err := interceptors.NewChain(s.interceptors...).ServerOptions()
		if err != nil {
			return coreerrs.WrapOperation(err, "build interceptor chain")
		}
		grpcOptions = append(grpcOptions, serverOpts...)
	}

	// Create gRPC server
	srv := grpc.NewServer(grpcOptions...)

	// Register handlers
	for _, h := range s.handlers {
		h.Register(srv, s.stopCh)
	}

	// Enable reflection if configured. Reflection exposes the full
	// service / method / field schema to any caller that can reach
	// the gRPC port — useful for development but a recon foothold in
	// production. Log a loud WARN at registration so operators can
	// grep for "reflection enabled" in their startup logs and catch
	// accidental production-enablement.
	if s.reflection {
		reflection.Register(srv)
		s.base.Logger().Warn("gRPC reflection enabled — service schema is publicly discoverable; disable in production")
	}

	// Use BaseServer.Start with gRPC-specific callback
	startErr := s.base.Start("grpc", func(ln net.Listener, errCh chan<- error) {
		go func() {
			// Recover panics that happen before grpc.Server.Serve has a
			// chance to install its own per-stream recovery (e.g. during
			// listener setup or option realization). Mirrors the
			// equivalent recovery in transport/http/server/http.go so a
			// panic in the spawn goroutine cannot bring down the process.
			defer panics.Handle(context.Background())

			if err := srv.Serve(ln); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				// Non-blocking send to avoid panic if channel is no longer being read
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	})
	if startErr != nil {
		srv.Stop()
		return startErr
	}

	s.grpc = srv
	return nil
}

// Shutdown gracefully stops the gRPC server.
// The shutdown process respects the provided context and will:
//  1. Attempt graceful shutdown (waiting for active connections)
//  2. Log progress warnings every 15 seconds
//  3. Force shutdown if context is canceled
//
// Returns nil on successful graceful shutdown.
// Returns context.DeadlineExceeded or context.Canceled if context expires,
// including while waiting for an in-flight [Server.Start] to finish.
// If the context expires before claiming the lifecycle semaphore, Done may
// remain open and the server is untouched; call Shutdown again to finish cleanup.
// After claiming the semaphore, even a never-started server closes Done.
// This method is safe to call multiple times (idempotent).
func (s *Server) Shutdown(ctx context.Context) error {
	select {
	case s.lifecycle <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.lifecycle }()
	if s.stopped {
		return nil
	}
	s.stopped = true
	close(s.stopCh)
	srv := s.grpc

	return s.base.GracefulShutdown(ctx, func(ctx context.Context) error {
		if srv == nil {
			return nil
		}

		// Close listener first to prevent new connections
		if s.Listener() != nil {
			_ = s.Listener().Close()
		}

		// ch is intentionally never closed: on timeout we return while the
		// goroutine may still be inside GracefulStop, and its later send
		// would panic on a closed channel. The buffer of one keeps that
		// send non-blocking; GC reclaims the channel.
		ch := make(chan struct{}, 1)

		go func(ch chan<- struct{}) {
			srv.GracefulStop()
			ch <- struct{}{}
		}(ch)

		select {
		case <-ctx.Done():
			srv.Stop()
			return ctx.Err()
		case <-ch:
			return nil
		}
	})
}

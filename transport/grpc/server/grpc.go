// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"context"
	"net"
	"slices"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

// GRPCServer is the contract for a managed gRPC server. It extends
// [baseserver.Server] with handler and interceptor registration.
// All methods are safe for concurrent use after the server has started.
type GRPCServer interface {
	baseserver.Server

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
// reflection, and interceptor chaining. It embeds [baseserver.BaseServer]
// for listener management and graceful shutdown.
//
// Typical sequence: [New] -> RegisterHandlers -> RegisterInterceptors -> [Server.Start] -> [Server.Shutdown].
// All exported methods are safe for concurrent use. A stopped Server cannot be restarted.
type Server struct {
	*baseserver.BaseServer

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
//   - Default address :50051
//   - No custom gRPC options
//
// Returns a configured Server instance ready for service registration and startup.
// The error return is reserved for future use and currently always returns nil.
func New(opts ...Option) (*Server, error) {
	// Create config with defaults
	cfg := newOptions(opts...)

	// Create server
	srv := &Server{
		BaseServer:  baseserver.NewBaseServer(cfg.baseOpts...),
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
	s.handlers = slices.Collect(coreslices.Filter(handlers, func(h Handler) bool {
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

// IsStarted returns true if the server has been started and is currently running.
func (s *Server) IsStarted() bool {
	return s.BaseServer.IsStarted()
}

// IsStopped returns true if the server has been stopped.
func (s *Server) IsStopped() bool {
	return s.IsShutdown()
}

// Start starts the gRPC server and begins accepting connections.
// It performs the following operations in sequence:
//  1. Validates server is not already started
//  2. Registers reflection service if enabled
//  3. Creates TCP listener if none provided
//  4. Starts serving in a background goroutine
//  5. Waits for startup verification (2 seconds)
//
// Returns ErrServerAlreadyStarted if called multiple times and
// ErrServerClosed once the server has been shut down; the running server is
// left untouched in both cases. A failed start may be retried.
// Returns network errors if listener creation or binding fails.
// This method is non-blocking after successful startup verification.
func (s *Server) Start() (err error) {
	s.lifecycle <- struct{}{}
	defer func() { <-s.lifecycle }()

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
		s.Logger().Warn("gRPC reflection enabled — service schema is publicly discoverable; disable in production")
	}

	// Use BaseServer.Start with gRPC-specific callback
	startErr := s.BaseServer.Start("grpc", func(ln net.Listener, errCh chan<- error) {
		go func() {
			// Recover panics that happen before grpc.Server.Serve has a
			// chance to install its own per-stream recovery (e.g. during
			// listener setup or option realization). Mirrors the
			// equivalent recovery in transport/http/server/http.go so a
			// panic in the spawn goroutine cannot bring down the process.
			defer panics.Handle(context.Background())

			if err := srv.Serve(ln); err != nil {
				// Non-blocking send to avoid panic if channel is no longer being read
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	})
	if startErr != nil {
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
// This method is safe to call multiple times (idempotent); it is a no-op on
// a server that is not running.
func (s *Server) Shutdown(ctx context.Context) error {
	select {
	case s.lifecycle <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	srv := s.grpc
	if srv == nil || s.stopped {
		<-s.lifecycle
		return nil
	}
	s.stopped = true
	<-s.lifecycle

	return s.GracefulShutdown(ctx, func(ctx context.Context) error {
		close(s.stopCh)

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

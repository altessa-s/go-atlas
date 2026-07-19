// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/transport/internal/timeouts"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	coretime "github.com/altessa-s/go-atlas/core/time"
)

// Sentinel errors returned by [BaseServer] lifecycle methods.
var (
	// ErrServerAlreadyStarted is returned by [BaseServer.Start] when the
	// server has already been started. The atomic flag prevents double-start.
	ErrServerAlreadyStarted = errors.New("server already started")

	// ErrServerClosed signals that the server has been shut down. Returned
	// by [BaseServer.GracefulShutdown] when shutdown completes.
	ErrServerClosed = errors.New("server closed")

	// ErrInvalidConfiguration indicates that a required option (e.g.
	// address via [WithAddress]) is missing or invalid.
	ErrInvalidConfiguration = errors.New("invalid server configuration")
)

// BaseServer provides listener management, TLS wrapping, startup
// verification, and graceful-shutdown orchestration shared by the HTTP
// and gRPC server implementations. Lifecycle state is tracked with
// atomic flags so that Start and GracefulShutdown are safe to call from
// any goroutine.
type BaseServer struct {
	started  atomic.Bool
	shutdown atomic.Bool
	options  *options
}

// Server is the common interface implemented by HTTP and gRPC server
// wrappers. [GetBase] provides access to the underlying [BaseServer] for
// shared lifecycle operations.
type Server interface {
	Start() error
	Shutdown(ctx context.Context) error
	GetBase() *BaseServer
}

// StartFunc performs protocol-specific startup logic. The implementation
// must launch its serving loop (typically in a new goroutine) and send
// any early startup error to errCh. If no error is sent within the
// startup-verification timeout, the server is considered healthy.
type StartFunc func(ln net.Listener, errCh chan<- error)

// ShutdownFunc performs protocol-specific graceful shutdown. The context
// carries the deadline configured by [timeouts.Config.ShutdownGraceful];
// implementations should respect it to avoid blocking indefinitely.
type ShutdownFunc func(ctx context.Context) error

// GetBase returns the BaseServer instance.
func (s *BaseServer) GetBase() *BaseServer {
	return s
}

// NewBaseServer creates a new [BaseServer] with the given options.
// Panics if no address is configured (use [WithAddress]).
func NewBaseServer(opt ...Option) *BaseServer {
	s := &BaseServer{
		options: newOptions(opt...),
	}

	// Validate that address was provided
	panics.Must(s.options.address != "", "server: address is required, use WithAddress option")

	return s
}

// Listen returns the pre-configured listener if one was provided via
// [WithListener], otherwise creates a new TCP listener on [BaseServer.Address].
// When TLS is configured, the raw listener is wrapped with [tls.NewListener].
func (s *BaseServer) Listen() (net.Listener, error) {
	if s.Listener() != nil {
		return s.Listener(), nil
	}

	if s.Address() == "" {
		return nil, errors.New("server address not configured")
	}

	ln, err := net.Listen("tcp", s.Address()) //nolint:noctx
	if err != nil {
		return nil, coreerrs.WrapOperationWithContext(err, "listen", s.Address())
	}

	if s.options.tlsConfig != nil {
		ln = tls.NewListener(ln, s.options.tlsConfig)
	}

	s.options.listener = ln
	return ln, nil
}

// validateStart checks if the server can be started.
func (s *BaseServer) validateStart() error {
	if !s.started.CompareAndSwap(false, true) {
		return ErrServerAlreadyStarted
	}
	return nil
}

// Start starts the server using the provided protocol-specific startup function.
// It validates state, creates a listener, and waits for startup verification.
//
// Example:
//
//	s.BaseServer.Start("http", func(ln net.Listener, errCh chan<- error) {
//	    go func() {
//	        if err := s.httpServer.Serve(ln); err != nil {
//	            select {
//	            case errCh <- err:
//	            default:
//	            }
//	        }
//	    }()
//	})
func (s *BaseServer) Start(protocol string, startFn StartFunc) error {
	// Validate server can be started
	if err := s.validateStart(); err != nil {
		return err
	}

	// Create listener
	ln, err := s.Listen()
	if err != nil {
		s.started.Store(false) // Reset flag on error
		return err
	}

	// Log startup
	attrs := []any{
		slog.String("protocol", s.Protocol(protocol)),
		slog.String("addr", ln.Addr().String()),
	}

	attrs = coreslices.AppendNonEmpty(attrs, "name", s.options.name)

	s.Logger().Info("server starting", attrs...)

	// Create error channel for startup verification
	chErr := make(chan error, 1)

	// Execute protocol-specific startup logic
	startFn(ln, chErr)

	// Wait for startup verification or early error
	timer := time.NewTimer(s.options.timeouts.StartupVerification)
	defer coretime.TimerStopAndDrain(timer)

	select {
	case err := <-chErr:
		s.started.Store(false) // Reset flag on error
		return err
	case <-timer.C:
		s.Logger().Info("server started successfully", "address", ln.Addr().String())
	}

	return nil
}

// WaitForShutdown blocks until ctx is canceled, returning ctx.Err().
// Use this in the main goroutine to keep the process alive until a
// shutdown signal arrives.
func (s *BaseServer) WaitForShutdown(ctx context.Context) error {
	// If context is already done, return immediately
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Wait for context cancellation or timeout
	<-ctx.Done()
	return ctx.Err()
}

// GracefulShutdown invokes shutdownFunc in a separate goroutine and
// periodically logs a warning while it is still running (interval
// controlled by [timeouts.Config.ShutdownWarning]). If ctx is canceled
// before shutdownFunc returns, the server is force-stopped and ctx.Err()
// is returned. A no-op if the server is not currently started.
func (s *BaseServer) GracefulShutdown(ctx context.Context, shutdownFunc ShutdownFunc) error {
	if !s.started.CompareAndSwap(true, false) {
		return nil
	}

	s.shutdown.Store(true)

	ch := make(chan error, 1)
	// We don't close ch here because we can't guarantee the goroutine won't write to it
	// after we return if context is canceled. Let GC handle it.

	ticker := time.NewTicker(s.options.timeouts.ShutdownWarning)
	defer ticker.Stop()

	shutdownStart := time.Now()

	go func() {
		defer panics.Handle(ctx)
		ch <- shutdownFunc(ctx)
	}()

	for {
		select {
		case <-ctx.Done():
			s.Logger().Warn("shutdown context canceled, forcing stop")
			return ctx.Err()
		case <-ticker.C:
			s.Logger().Warn("server still shutting down", "elapsed", time.Since(shutdownStart))
		case err := <-ch:
			if err != nil {
				s.Logger().Error("server shutdown error", "error", err)
				return err
			}
			s.Logger().Info("server shutdown complete")
			return nil
		}
	}
}

// Logger returns the configured [slog.Logger].
func (s *BaseServer) Logger() *slog.Logger {
	return s.options.logger
}

// Protocol returns protocol name with "+tls" suffix if TLS is configured.
func (s *BaseServer) Protocol(baseProtocol string) string {
	if s.HasTLS() {
		const tlsSuffix = "+tls"
		return corestrings.BuildString(func(b *strings.Builder) {
			b.Grow(len(baseProtocol) + len(tlsSuffix))
			b.WriteString(baseProtocol)
			b.WriteString(tlsSuffix)
		})
	}
	return baseProtocol
}

// HasTLS returns true if TLS is configured.
func (s *BaseServer) HasTLS() bool {
	return s.options.tlsConfig != nil
}

// HasListener returns true if a listener is set.
func (s *BaseServer) HasListener() bool {
	return !nilcheck.IsNil(s.Listener())
}

// Address returns the listener's actual address if a listener is active,
// otherwise the configured address string. When the configured address
// uses port :0, this returns the OS-assigned port after [Listen].
func (s *BaseServer) Address() string {
	if !nilcheck.IsNil(s.Listener()) {
		return s.Listener().Addr().String()
	}
	return s.options.address
}

// Listener returns the listener set by [WithListener] or created by [Listen].
// May be nil before [Listen] is called.
func (s *BaseServer) Listener() net.Listener {
	return s.options.listener
}

// TLSConfig returns TLS configuration.
func (s *BaseServer) TLSConfig() *tls.Config {
	return s.options.tlsConfig
}

// Name returns server name.
func (s *BaseServer) Name() string {
	return s.options.name
}

// Timeouts returns timeout configuration.
func (s *BaseServer) Timeouts() timeouts.Config {
	return s.options.timeouts
}

// IsStarted returns true if server is started.
func (s *BaseServer) IsStarted() bool {
	return s.started.Load()
}

// IsShutdown returns true if server is shut down.
func (s *BaseServer) IsShutdown() bool {
	return s.shutdown.Load()
}

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
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/transport/internal/timeouts"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Sentinel errors returned by [BaseServer] lifecycle methods.
var (
	// ErrServerAlreadyStarted is returned by [BaseServer.Start] when the
	// server has already been started.
	ErrServerAlreadyStarted = errors.New("server already started")

	// ErrServerClosed is returned by [BaseServer.Start] after Done has closed.
	ErrServerClosed = errors.New("server closed")

	// ErrInvalidConfiguration indicates that a required option (e.g.
	// address via [WithAddress]) is missing or invalid.
	ErrInvalidConfiguration = errors.New("invalid server configuration")
)

// BaseServer provides listener management, TLS wrapping, startup
// verification, and graceful-shutdown orchestration shared by the HTTP
// and gRPC server implementations. Lifecycle state is tracked with
// a lifecycle mutex and atomic flags so Start and GracefulShutdown are safe from
// any goroutine.
type BaseServer struct {
	cleanupPending  atomic.Bool
	shutdownWaiting atomic.Bool // test-only instrumentation: reports shutdown contention on lifecycle
	started         atomic.Bool
	shutdown        atomic.Bool
	options         *options
	listenerOwned   bool // listener was created by Listen, rather than supplied by WithListener

	// lifecycle serializes startup, shutdown, and Serve failure recording.
	lifecycle     sync.Mutex
	done          chan struct{}
	doneOnce      sync.Once
	watchStop     chan struct{}
	watchStopOnce sync.Once
	watchers      sync.WaitGroup
	errMu         sync.RWMutex
	serveErr      error
}

// Server is the common interface implemented by HTTP and gRPC server
// wrappers. [GetBase] provides access to the underlying [BaseServer] for
// shared lifecycle operations.
type Server interface {
	Start() error
	Shutdown(ctx context.Context) error
	GetBase() *BaseServer
	// Done closes exactly once when a post-start Serve error is recorded or
	// GracefulShutdown completes, including cancellation and not-started paths.
	// Early Start errors leave Done open; see Start for retry limitations. A wrapper
	// may abort Shutdown before claiming its lifecycle; Done then remains open.
	Done() <-chan struct{}
	// Err returns the first recorded post-start Serve error, or nil before a
	// failure and after normal graceful shutdown. Early Start errors are not stored.
	Err() error
}

// StartFunc performs protocol-specific startup logic. The implementation
// must launch its serving loop (typically in a new goroutine) and send
// any non-normal Serve error to errCh, including errors after startup. If no
// error is sent within the startup-verification timeout, the server is
// considered healthy. The channel must not be closed.
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
		options:   newOptions(opt...),
		done:      make(chan struct{}),
		watchStop: make(chan struct{}),
	}

	// Validate that address was provided
	panics.Must(s.options.address != "", "server: address is required, use WithAddress option")

	return s
}

// Done closes exactly once when a post-start Serve error is recorded or
// GracefulShutdown completes, including cancellation and not-started paths.
// Early Start errors leave Done open; see Start for retry limitations.
func (s *BaseServer) Done() <-chan struct{} { return s.done }

// Err returns the first recorded post-start Serve error, or nil before a
// failure and after normal graceful shutdown. Early Start errors are not stored.
func (s *BaseServer) Err() error {
	s.errMu.RLock()
	defer s.errMu.RUnlock()
	return s.serveErr
}

func (s *BaseServer) closeDone()   { s.doneOnce.Do(func() { close(s.done) }) }
func (s *BaseServer) stopWatcher() { s.watchStopOnce.Do(func() { close(s.watchStop) }) }

func (s *BaseServer) watch(errCh <-chan error) {
	select {
	case err := <-errCh:
		s.lifecycle.Lock()
		defer s.lifecycle.Unlock()
		if err == nil || s.shutdown.Load() {
			return
		}
		s.errMu.Lock()
		if s.serveErr != nil {
			s.errMu.Unlock()
			return
		}
		s.serveErr = err
		s.errMu.Unlock()
		s.started.Store(false)
		s.Logger().Error("server serve error", "error", err)
		s.closeDone()
	case <-s.watchStop:
	}
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
	s.listenerOwned = true
	return ln, nil
}

// CanStart checks lifecycle state without reserving startup. Start performs
// the authoritative validation under the lifecycle lock.
func (s *BaseServer) CanStart() error {
	select {
	case <-s.done:
		return ErrServerClosed
	default:
	}
	if s.started.Load() {
		return ErrServerAlreadyStarted
	}
	return nil
}

// validateStart checks if the server can be started.
func (s *BaseServer) validateStart() error {
	select {
	case <-s.done:
		return ErrServerClosed
	default:
	}
	if !s.started.CompareAndSwap(false, true) {
		return ErrServerAlreadyStarted
	}
	return nil
}

// Start starts the server using the provided protocol-specific startup function.
// It validates state, creates a listener, and waits for startup verification.
// Early errors are returned synchronously and leave Done open. Retrying is
// meaningful for listen/bind or pre-Serve errors. Auto-created listeners are
// closed and cleared after an early startup error so retries open a fresh socket.
// A WithListener listener may be consumed by a failed Serve and is never replaced.
// Once Done has closed, Start returns ErrServerClosed; create a new server to restart.
//
// Example:
//
//	s.BaseServer.Start("http", func(ln net.Listener, errCh chan<- error) {
//	    go func() {
//	        if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
//	            select {
//	            case errCh <- err:
//	            default:
//	            }
//	        }
//	    }()
//	})
func (s *BaseServer) Start(protocol string, startFn StartFunc) error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
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
	defer timer.Stop()

	select {
	case err := <-chErr:
		if s.listenerOwned {
			_ = ln.Close()
			s.options.listener = nil
			s.listenerOwned = false
		}
		s.started.Store(false) // Reset flag on error
		return err
	case <-timer.C:
		s.Logger().Info("server started successfully", "address", ln.Addr().String())
	}

	s.cleanupPending.Store(true)
	s.watchers.Go(func() { s.watch(chErr) })
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
// before shutdownFunc returns, ctx.Err() is returned; shutdownFunc is
// responsible for protocol-specific force-stop behavior, which may finish after
// GracefulShutdown returns. Pending protocol cleanup runs once, even after a
// Serve failure.
// Without pending cleanup, it only stops observation and closes Done.
// It may first wait for an in-flight Start (including startup verification)
// holding the lifecycle mutex. Context cancellation cannot interrupt that wait.
func (s *BaseServer) GracefulShutdown(ctx context.Context, shutdownFunc ShutdownFunc) error {
	if !s.lifecycle.TryLock() {
		s.shutdownWaiting.Store(true)
		s.lifecycle.Lock()
		s.shutdownWaiting.Store(false)
	}
	defer func() {
		s.stopWatcher()
		s.closeDone()
		s.lifecycle.Unlock()
		s.watchers.Wait()
	}()
	s.shutdown.Store(true)
	s.started.Store(false)
	if !s.cleanupPending.CompareAndSwap(true, false) {
		return nil
	}

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

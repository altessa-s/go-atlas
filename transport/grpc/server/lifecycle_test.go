// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc_test

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
	"github.com/altessa-s/go-atlas/transport/internal/timeouts"
	stdGrpc "google.golang.org/grpc"
)

// fastVerify keeps the startup-verification wait of successful starts short.
const fastVerify = 10 * time.Millisecond

// countingHandler counts how many gRPC servers it was registered with and,
// when gate is non-nil, signals entered and blocks inside Register until
// gate is closed — i.e. while Start is in its critical section.
type countingHandler struct {
	calls   atomic.Int32
	entered chan struct{}
	gate    chan struct{}
}

func (h *countingHandler) Register(_ *stdGrpc.Server, _ <-chan struct{}) {
	h.calls.Add(1)
	if h.gate != nil {
		h.entered <- struct{}{}
		<-h.gate
	}
}

// newLifecycleServer builds a server on an ephemeral listener with the given
// startup verification window.
func newLifecycleServer(t *testing.T, verify time.Duration, handlers ...grpcserver.Handler) (*grpcserver.Server, net.Listener) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv, err := grpcserver.New(grpcserver.WithBaseOptions(
		baseserver.WithListener(ln),
		baseserver.WithTimeouts(timeouts.Config{
			StartupVerification: verify,
			ShutdownGraceful:    time.Minute,
			ShutdownWarning:     time.Minute,
		}),
	))
	require.NoError(t, err)
	srv.RegisterHandlers(handlers...)
	return srv, ln
}

// TestServer_Start_RepeatedKeepsRunningServer pins that a second Start is
// rejected before a replacement server is built: before the fix it swapped
// the server pointer, so Shutdown stopped the replacement and the original
// stream stayed open forever.
func TestServer_Start_RepeatedKeepsRunningServer(t *testing.T) {
	t.Parallel()

	blocking := blockingStreamHandler{started: make(chan struct{}, 1)}
	counter := &countingHandler{}
	srv, ln := newLifecycleServer(t, fastVerify, blocking, counter)
	require.NoError(t, srv.Start())

	conn, err := stdGrpc.NewClient(ln.Addr().String(), stdGrpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	stream, err := conn.NewStream(t.Context(), &stdGrpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/test.Blocking/Block")
	require.NoError(t, err)
	require.NoError(t, stream.SendMsg(&emptypb.Empty{}))
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "stream handler did not start")
	}

	require.ErrorIs(t, srv.Start(), baseserver.ErrServerAlreadyStarted)
	require.Equal(t, int32(1), counter.calls.Load())

	// The open stream blocks GracefulStop, so the deadline forces Stop on
	// the original server, which must terminate the stream.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, srv.Shutdown(ctx), context.DeadlineExceeded)

	recvErr := make(chan error, 1)
	go func() { recvErr <- stream.RecvMsg(&emptypb.Empty{}) }()
	select {
	case err := <-recvErr:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "original server kept serving the stream after Shutdown")
	}
}

// TestServer_Start_Concurrent releases several Start calls at once: exactly
// one wins and handlers are registered with a single server.
func TestServer_Start_Concurrent(t *testing.T) {
	t.Parallel()

	counter := &countingHandler{}
	srv, _ := newLifecycleServer(t, fastVerify, counter)
	t.Cleanup(func() { _ = srv.Shutdown(context.WithoutCancel(t.Context())) })

	const callers = 8
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			<-start
			errs <- srv.Start()
		})
	}
	close(start)
	wg.Wait()
	close(errs)

	var ok int
	for err := range errs {
		if err == nil {
			ok++
			continue
		}
		require.ErrorIs(t, err, baseserver.ErrServerAlreadyStarted)
	}
	require.Equal(t, 1, ok)
	require.Equal(t, int32(1), counter.calls.Load())
}

// TestServer_Start_AfterShutdown pins the documented "a stopped Server
// cannot be restarted" contract.
func TestServer_Start_AfterShutdown(t *testing.T) {
	t.Parallel()

	counter := &countingHandler{}
	srv, _ := newLifecycleServer(t, fastVerify, counter)
	require.NoError(t, srv.Start())
	require.NoError(t, srv.Shutdown(t.Context()))

	require.ErrorIs(t, srv.Start(), baseserver.ErrServerClosed)
	require.Equal(t, int32(1), counter.calls.Load())
	require.NoError(t, srv.Shutdown(t.Context()))
}

// TestServer_Shutdown_DuringStart holds Start inside its critical section
// (handler registration) and checks that Shutdown cannot claim the server
// meanwhile: an expired context gives up with ctx.Err(), and a live one
// stops the server once Start completes. Before the fix Shutdown saw a
// not-yet-started server, returned nil, and the server kept running.
func TestServer_Shutdown_DuringStart(t *testing.T) {
	t.Parallel()

	counter := &countingHandler{entered: make(chan struct{}, 1), gate: make(chan struct{})}
	srv, _ := newLifecycleServer(t, fastVerify, counter)

	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start() }()
	<-counter.entered

	expired, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, srv.Shutdown(expired), context.Canceled)

	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- srv.Shutdown(t.Context()) }()

	close(counter.gate)
	require.NoError(t, <-startErr)
	require.NoError(t, <-shutdownErr)

	require.True(t, srv.IsStopped())
	require.False(t, srv.IsStarted())
	require.ErrorIs(t, srv.Start(), baseserver.ErrServerClosed)
}

// TestServer_Start_FailureIsRetryable checks that a start whose Serve fails
// leaves the server unclaimed: Shutdown stays a no-op and a later Start is
// attempted again rather than being rejected.
func TestServer_Start_FailureIsRetryable(t *testing.T) {
	t.Parallel()

	counter := &countingHandler{}
	// A long verification window: the failing Serve reports well before it,
	// so the outcome does not depend on goroutine scheduling.
	srv, ln := newLifecycleServer(t, time.Minute, counter)
	require.NoError(t, ln.Close())

	err := srv.Start()
	require.Error(t, err)
	require.NotErrorIs(t, err, baseserver.ErrServerAlreadyStarted)

	require.NoError(t, srv.Shutdown(t.Context()))
	require.False(t, srv.IsStopped())

	err = srv.Start()
	require.Error(t, err)
	require.NotErrorIs(t, err, baseserver.ErrServerAlreadyStarted)
	require.NotErrorIs(t, err, baseserver.ErrServerClosed)
	require.Equal(t, int32(2), counter.calls.Load())
}

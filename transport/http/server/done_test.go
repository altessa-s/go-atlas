// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
	"github.com/altessa-s/go-atlas/transport/internal/timeouts"

	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

func TestServer_DoneServeFailure(t *testing.T) {
	ln := &doneListener{closed: make(chan struct{})}
	srv, err := New(WithRouter(std.New()), WithBaseOptions(baseserver.WithListener(ln), baseserver.WithTimeouts(timeouts.Config{
		StartupVerification: 10 * time.Millisecond, ShutdownWarning: time.Second,
	})))
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-srv.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done did not close after listener failure")
	}
	if !errors.Is(srv.Err(), net.ErrClosed) {
		t.Fatalf("Err = %v, want net.ErrClosed", srv.Err())
	}
	if srv.IsStarted() {
		t.Fatal("server still started")
	}
	if err := srv.Start(); !errors.Is(err, baseserver.ErrServerClosed) {
		t.Fatalf("Start after failure = %v", err)
	}
	first := srv.Err()
	if err := srv.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-srv.stopCh:
	default:
		t.Fatal("Shutdown did not close stopCh")
	}
	if srv.Err() != first {
		t.Fatal("Shutdown changed Serve error")
	}
}

func TestServer_DoneNormalShutdown(t *testing.T) {
	ln := &doneListener{closed: make(chan struct{})}
	srv, err := New(WithRouter(std.New()), WithBaseOptions(baseserver.WithListener(ln), baseserver.WithTimeouts(timeouts.Config{
		StartupVerification: 10 * time.Millisecond, ShutdownWarning: time.Second,
	})))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	if err := srv.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-srv.Done():
	default:
		t.Fatal("Shutdown did not close Done")
	}
	if err := srv.Start(); !errors.Is(err, baseserver.ErrServerClosed) {
		t.Fatalf("Start after shutdown = %v", err)
	}
	if srv.Err() != nil {
		t.Fatalf("Err = %v after normal shutdown", srv.Err())
	}
}

// doneListener delays a permanent Accept failure until the test closes it.
type doneListener struct {
	closed chan struct{}
	once   sync.Once
}

func (l *doneListener) Accept() (net.Conn, error) { <-l.closed; return nil, net.ErrClosed }
func (l *doneListener) Close() error              { l.once.Do(func() { close(l.closed) }); return nil }
func (*doneListener) Addr() net.Addr              { return &net.TCPAddr{} }

// retryListener deliberately tolerates the first Close from a failed Serve,
// then permits a successful retry. Ordinary WithListener listeners are consumed.
type retryListener struct {
	doneListener
	failed bool
	closes int
}

func (l *retryListener) Accept() (net.Conn, error) {
	if !l.failed {
		l.failed = true
		return nil, errors.New("early failure")
	}
	return l.doneListener.Accept()
}
func (l *retryListener) Close() error {
	l.closes++
	if l.closes == 1 {
		return nil
	}
	return l.doneListener.Close()
}

type retryHandler struct{ calls int }

func (h *retryHandler) Register(r RouteRegistrar, _ <-chan struct{}) {
	h.calls++
	r.Handle("/retry", func(writer.ReadWriter) {})
}

func TestServer_EarlyFailureRetry(t *testing.T) {
	ln := &retryListener{doneListener: doneListener{closed: make(chan struct{})}}
	srv, err := New(WithRouter(std.New()), WithBaseOptions(baseserver.WithListener(ln), baseserver.WithTimeouts(timeouts.Config{
		StartupVerification: 50 * time.Millisecond, ShutdownWarning: time.Second,
	})))
	if err != nil {
		t.Fatal(err)
	}
	handler := &retryHandler{}
	srv.RegisterHandlers(handler)
	if err := srv.Start(); err == nil {
		t.Fatal("expected early error")
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	if err := srv.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); !errors.Is(err, baseserver.ErrServerClosed) {
		t.Fatalf("Start = %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("handler registrations = %d", handler.calls)
	}
}

func TestServer_ConcurrentStart(t *testing.T) {
	ln := &doneListener{closed: make(chan struct{})}
	srv, err := New(WithRouter(std.New()), WithBaseOptions(
		baseserver.WithListener(ln),
		baseserver.WithTimeouts(timeouts.Config{
			StartupVerification: 50 * time.Millisecond, ShutdownWarning: time.Second,
		}),
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	ready := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-ready
			results <- srv.Start()
		}()
	}
	close(ready)
	var successes, alreadyStarted int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			successes++
		case errors.Is(err, baseserver.ErrServerAlreadyStarted):
			alreadyStarted++
		default:
			t.Fatalf("Start = %v", err)
		}
	}
	if successes != 1 || alreadyStarted != 1 {
		t.Fatalf("successes=%d alreadyStarted=%d, want 1 each", successes, alreadyStarted)
	}
	if err := srv.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ln.closed:
	default:
		t.Fatal("Shutdown left serving listener open")
	}
}

func TestServer_CanceledShutdownClosesActiveConnection(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprintf("deadline=%t", deadline), func(t *testing.T) {
			r := std.New()
			entered, exited, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			r.HandleFunc("/", func(_ http.ResponseWriter, req *http.Request) {
				close(entered)
				defer close(exited)
				select {
				case <-req.Context().Done():
				case <-release:
				}
			})
			srv, err := New(WithRouter(r), WithBaseOptions(
				baseserver.WithAddress("127.0.0.1:0"),
				baseserver.WithTimeouts(timeouts.Config{
					StartupVerification: 10 * time.Millisecond, ShutdownWarning: time.Second,
				}),
			))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				close(release)
				_ = srv.Shutdown(context.Background())
			})
			if err := srv.Start(); err != nil {
				t.Fatal(err)
			}
			conn, err := net.DialTimeout("tcp", srv.Address(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if _, err := fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("handler did not start")
			}
			ctx, cancel := context.WithCancel(t.Context())
			want := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				want = context.DeadlineExceeded
			} else {
				cancel()
			}
			defer cancel()
			if err := srv.Shutdown(ctx); !errors.Is(err, want) {
				t.Fatalf("Shutdown = %v, want %v", err, want)
			}
			select {
			case <-exited:
			case <-time.After(time.Second):
				t.Fatal("force-close did not cancel active request")
			}
			if srv.Err() != nil {
				t.Fatalf("Err = %v after cancellation", srv.Err())
			}
		})
	}
}

// acceptedFailureListener accepts one connection before a permanent Serve error.
type acceptedFailureListener struct {
	conn net.Conn
}

func (l *acceptedFailureListener) Accept() (net.Conn, error) {
	if l.conn != nil {
		conn := l.conn
		l.conn = nil
		return conn, nil
	}
	return nil, net.ErrClosed
}
func (*acceptedFailureListener) Close() error   { return nil }
func (*acceptedFailureListener) Addr() net.Addr { return &net.TCPAddr{} }

func TestServer_EarlyFailureClosesAcceptedConnection(t *testing.T) {
	accepted, client := net.Pipe()
	t.Cleanup(func() { _ = accepted.Close() })
	t.Cleanup(func() { _ = client.Close() })
	ln := &acceptedFailureListener{conn: accepted}
	srv, err := New(WithRouter(std.New()), WithBaseOptions(
		baseserver.WithListener(ln),
		baseserver.WithTimeouts(timeouts.Config{StartupVerification: 5 * time.Second}),
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Start = %v, want net.ErrClosed", err)
	}
	// Close must have closed the accepted connection before Start returns.
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if _, err := client.Read(make([]byte, 1)); !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Read = %v, want EOF or ErrClosedPipe from orphaned server cleanup", err)
	}
	select {
	case <-srv.Done():
		t.Fatal("early failure closed Done")
	default:
	}
}

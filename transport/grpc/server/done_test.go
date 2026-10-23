// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/transport/internal/timeouts"

	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
	stdgrpc "google.golang.org/grpc"
)

func TestServer_DoneServeFailure(t *testing.T) {
	ln := &doneListener{closed: make(chan struct{})}
	srv, err := New(WithBaseOptions(baseserver.WithListener(ln), baseserver.WithTimeouts(timeouts.Config{
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
	srv, err := New(WithBaseOptions(baseserver.WithListener(ln), baseserver.WithTimeouts(timeouts.Config{
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

func TestServer_DoneNeverStarted(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-srv.Done():
	default:
		t.Fatal("Done remains open")
	}
	if !srv.IsStopped() || srv.Err() != nil {
		t.Fatalf("stopped=%v Err=%v", srv.IsStopped(), srv.Err())
	}
}

type captureServerHandler struct{ server *stdgrpc.Server }

func (h *captureServerHandler) Register(server *stdgrpc.Server, _ <-chan struct{}) {
	h.server = server
}

func TestServer_EarlyFailureStopsCreatedServer(t *testing.T) {
	ln := &doneListener{closed: make(chan struct{})}
	_ = ln.Close()
	srv, err := New(WithListener(ln), WithStartupVerification(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	handler := &captureServerHandler{}
	srv.RegisterHandlers(handler)
	if err := srv.Start(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Start = %v, want net.ErrClosed", err)
	}
	if handler.server == nil {
		t.Fatal("underlying gRPC server was not created")
	}
	t.Cleanup(handler.server.Stop)
	// Serve on a stopped server returns ErrServerStopped before Accept.
	if err := handler.server.Serve(ln); !errors.Is(err, stdgrpc.ErrServerStopped) {
		t.Fatalf("Serve = %v, want ErrServerStopped after early failure", err)
	}
	select {
	case <-srv.Done():
		t.Fatal("early failure closed Done")
	default:
	}
}

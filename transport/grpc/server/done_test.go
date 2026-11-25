// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/lifecycle"
)

func TestServer_DoneServeFailure(t *testing.T) {
	t.Parallel()
	ln := &doneListener{closed: make(chan struct{})}
	srv, err := New(WithListener(ln), WithLifecycleTimeouts(lifecycle.Timeouts{
		StartupVerification: 10 * time.Millisecond, ShutdownWarning: time.Second,
	}))
	require.NoError(t, err)
	require.NoError(t, srv.Start())
	require.NoError(t, ln.Close())
	select {
	case <-srv.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done did not close after listener failure")
	}
	require.ErrorIs(t, srv.Err(), net.ErrClosed)
	if srv.IsStarted() {
		t.Fatal("server still started")
	}
	require.ErrorIs(t, srv.Start(), ErrServerClosed)
	first := srv.Err()
	require.NoError(t, srv.Shutdown(t.Context()))
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
	t.Parallel()
	ln := &doneListener{closed: make(chan struct{})}
	srv, err := New(WithListener(ln), WithLifecycleTimeouts(lifecycle.Timeouts{
		StartupVerification: 10 * time.Millisecond, ShutdownWarning: time.Second,
	}))
	require.NoError(t, err)
	defer ln.Close()
	require.NoError(t, srv.Start())
	require.NoError(t, srv.Shutdown(t.Context()))
	select {
	case <-srv.Done():
	default:
		t.Fatal("Shutdown did not close Done")
	}
	require.ErrorIs(t, srv.Start(), ErrServerClosed)
	require.NoError(t, srv.Err())
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
	t.Parallel()
	srv, err := New()
	require.NoError(t, err)
	require.NoError(t, srv.Shutdown(t.Context()))
	select {
	case <-srv.Done():
	default:
		t.Fatal("Done remains open")
	}
	require.True(t, srv.IsStopped())
	require.NoError(t, srv.Err())
}

func TestServer_EarlyFailureStopsCreatedServer(t *testing.T) {
	// Channelz tracks servers across the process, so keep this test serial.
	before := channelzServers(t)
	ln := &doneListener{closed: make(chan struct{})}
	_ = ln.Close()
	srv, err := New(WithListener(ln), WithStartupVerification(5*time.Second))
	require.NoError(t, err)
	require.ErrorIs(t, srv.Start(), net.ErrClosed)
	require.Equal(t, before, channelzServers(t))
	select {
	case <-srv.Done():
		t.Fatal("early failure closed Done")
	default:
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
	"github.com/altessa-s/go-atlas/transport/internal/timeouts"

	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	require.Equal(t, DefaultReadTimeout, opts.readTimeout)
	require.Equal(t, DefaultReadHeaderTimeout, opts.readHeaderTimeout,
		"ReadHeaderTimeout MUST default to a non-zero value (Slowloris defense, gosec G112)")
	require.Equal(t, DefaultWriteTimeout, opts.writeTimeout)
	require.Equal(t, DefaultIdleTimeout, opts.idleTimeout)
	require.Equal(t, DefaultMaxHeaderBytes, opts.maxHeaderBytes)
}

func TestWithReadTimeout(t *testing.T) {
	opts := newOptions(WithReadTimeout(30 * time.Second))
	require.Equal(t, 30*time.Second, opts.readTimeout)
}

func TestWithReadTimeout_Negative(t *testing.T) {
	opts := newOptions(WithReadTimeout(-1))
	require.Equal(t, DefaultReadTimeout, opts.readTimeout)
}

func TestWithReadHeaderTimeout(t *testing.T) {
	opts := newOptions(WithReadHeaderTimeout(7 * time.Second))
	require.Equal(t, 7*time.Second, opts.readHeaderTimeout)
}

func TestWithReadHeaderTimeout_Negative(t *testing.T) {
	opts := newOptions(WithReadHeaderTimeout(-1))
	require.Equal(t, DefaultReadHeaderTimeout, opts.readHeaderTimeout,
		"a negative value must NOT clobber the safe default with 0 — that would re-introduce the Slowloris hole")
}

func TestNew_PropagatesReadHeaderTimeoutToHTTPServer(t *testing.T) {
	// End-to-end check that the option actually reaches the underlying
	// *http.Server. Start the server, then assert via the internal field —
	// Slowloris defense lives in the stdlib field, not in our Server
	// struct copy.
	srv := newTestServer(t, "/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	require.NoError(t, srv.Start())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	require.Equal(t, DefaultReadHeaderTimeout, srv.http.ReadHeaderTimeout,
		"Server.Start must set http.Server.ReadHeaderTimeout — otherwise gosec G112 still triggers and Slowloris is wide open")
}

func TestWithWriteTimeout(t *testing.T) {
	opts := newOptions(WithWriteTimeout(30 * time.Second))
	require.Equal(t, 30*time.Second, opts.writeTimeout)
}

func TestWithWriteTimeout_Negative(t *testing.T) {
	opts := newOptions(WithWriteTimeout(-1))
	require.Equal(t, DefaultWriteTimeout, opts.writeTimeout)
}

func TestWithIdleTimeout(t *testing.T) {
	opts := newOptions(WithIdleTimeout(120 * time.Second))
	require.Equal(t, 120*time.Second, opts.idleTimeout)
}

func TestWithIdleTimeout_Negative(t *testing.T) {
	opts := newOptions(WithIdleTimeout(-1))
	require.Equal(t, DefaultIdleTimeout, opts.idleTimeout)
}

func TestWithMaxHeaderBytes(t *testing.T) {
	opts := newOptions(WithMaxHeaderBytes(2 << 20))
	require.Equal(t, 2<<20, opts.maxHeaderBytes)
}

func TestWithRouter_Nil(t *testing.T) {
	opts := newOptions(WithRouter(nil))
	require.Nil(t, opts.router)
}

func TestNew_NoRouter(t *testing.T) {
	_, err := New()
	require.Error(t, err)
}

func TestDefaultConstants(t *testing.T) {
	tests := []struct {
		name string
		got  time.Duration
	}{
		{"DefaultReadTimeout", DefaultReadTimeout},
		{"DefaultWriteTimeout", DefaultWriteTimeout},
		{"DefaultIdleTimeout", DefaultIdleTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Greater(t, tt.got, time.Duration(0))
		})
	}
	require.Greater(t, DefaultMaxHeaderBytes, 0)
}

// newTestServer creates an HTTP server bound to a random port with the
// given handler registered at the given pattern. It returns the server
// and the listener address (host:port).
func newTestServer(t *testing.T, pattern string, handler http.HandlerFunc) *Server {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	r := std.New()
	srv, err := New(
		WithRouter(r),
		WithBaseOptions(
			baseserver.WithAddress(ln.Addr().String()),
			baseserver.WithListener(ln),
			baseserver.WithTimeouts(timeouts.Config{
				StartupVerification: 200 * time.Millisecond,
				ShutdownGraceful:    5 * time.Second,
				ShutdownWarning:     5 * time.Second,
			}),
		),
	)
	if err != nil {
		ln.Close()
		require.NoError(t, err)
	}

	if handler != nil {
		srv.Handle(pattern, func(rw writer.ReadWriter) {
			handler(rw.ResponseWriter(), rw.Request())
		})
	}

	return srv
}

func TestShutdown(t *testing.T) {
	srv := newTestServer(t, "/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	require.NoError(t, srv.Start())

	// Verify the server is accepting connections.
	addr := srv.Address()
	resp, err := http.Get("http://" + addr + "/health")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Shutdown must complete without error (no double-close).
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	require.NoError(t, srv.Shutdown(ctx))

	// After shutdown, new connections must be refused.
	_, err = http.Get("http://" + addr + "/health")
	require.Error(t, err)
}

func TestShutdown_DrainsInFlightRequests(t *testing.T) {
	inHandler := make(chan struct{})
	releaseHandler := make(chan struct{})

	srv := newTestServer(t, "/slow", func(w http.ResponseWriter, r *http.Request) {
		close(inHandler)
		<-releaseHandler
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "done")
	})

	require.NoError(t, srv.Start())

	addr := srv.Address()

	// Start an in-flight request that blocks inside the handler.
	var (
		wg       sync.WaitGroup
		respBody string
		reqErr   error
	)
	wg.Go(func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			reqErr = err
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		respBody = string(b)
	})

	// Wait until the handler is entered.
	<-inHandler

	// Initiate shutdown while the request is in flight.
	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		shutdownDone <- srv.Shutdown(ctx)
	}()

	// Release the handler so the request completes.
	close(releaseHandler)
	wg.Wait()

	require.Nil(t, reqErr)
	require.Equal(t, "done", respBody)

	// Shutdown must return nil (no double-close error).
	require.NoError(t, <-shutdownDone)
}

func TestShutdown_NotStarted(t *testing.T) {
	srv := newTestServer(t, "", nil)

	// Shutdown on a server that was never started should be a no-op.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	require.NoError(t, srv.Shutdown(ctx))
}

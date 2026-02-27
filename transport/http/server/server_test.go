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

	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
	"github.com/altessa-s/go-atlas/transport/internal/timeouts"

	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	if opts.readTimeout != DefaultReadTimeout {
		t.Fatalf("readTimeout = %v, want %v", opts.readTimeout, DefaultReadTimeout)
	}
	if opts.writeTimeout != DefaultWriteTimeout {
		t.Fatalf("writeTimeout = %v, want %v", opts.writeTimeout, DefaultWriteTimeout)
	}
	if opts.idleTimeout != DefaultIdleTimeout {
		t.Fatalf("idleTimeout = %v, want %v", opts.idleTimeout, DefaultIdleTimeout)
	}
	if opts.maxHeaderBytes != DefaultMaxHeaderBytes {
		t.Fatalf("maxHeaderBytes = %d, want %d", opts.maxHeaderBytes, DefaultMaxHeaderBytes)
	}
}

func TestWithReadTimeout(t *testing.T) {
	opts := newOptions(WithReadTimeout(30 * time.Second))
	if opts.readTimeout != 30*time.Second {
		t.Fatalf("readTimeout = %v", opts.readTimeout)
	}
}

func TestWithReadTimeout_Negative(t *testing.T) {
	opts := newOptions(WithReadTimeout(-1))
	if opts.readTimeout != DefaultReadTimeout {
		t.Fatalf("negative should keep default, got %v", opts.readTimeout)
	}
}

func TestWithWriteTimeout(t *testing.T) {
	opts := newOptions(WithWriteTimeout(30 * time.Second))
	if opts.writeTimeout != 30*time.Second {
		t.Fatalf("writeTimeout = %v", opts.writeTimeout)
	}
}

func TestWithWriteTimeout_Negative(t *testing.T) {
	opts := newOptions(WithWriteTimeout(-1))
	if opts.writeTimeout != DefaultWriteTimeout {
		t.Fatalf("negative should keep default, got %v", opts.writeTimeout)
	}
}

func TestWithIdleTimeout(t *testing.T) {
	opts := newOptions(WithIdleTimeout(120 * time.Second))
	if opts.idleTimeout != 120*time.Second {
		t.Fatalf("idleTimeout = %v", opts.idleTimeout)
	}
}

func TestWithIdleTimeout_Negative(t *testing.T) {
	opts := newOptions(WithIdleTimeout(-1))
	if opts.idleTimeout != DefaultIdleTimeout {
		t.Fatalf("negative should keep default, got %v", opts.idleTimeout)
	}
}

func TestWithMaxHeaderBytes(t *testing.T) {
	opts := newOptions(WithMaxHeaderBytes(2 << 20))
	if opts.maxHeaderBytes != 2<<20 {
		t.Fatalf("maxHeaderBytes = %d", opts.maxHeaderBytes)
	}
}

func TestWithRouter_Nil(t *testing.T) {
	opts := newOptions(WithRouter(nil))
	if opts.router != nil {
		t.Fatal("nil router should not be set")
	}
}

func TestNew_NoRouter(t *testing.T) {
	_, err := New()
	if err == nil {
		t.Fatal("New() without router should error")
	}
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
			if tt.got <= 0 {
				t.Fatalf("%s = %v", tt.name, tt.got)
			}
		})
	}
	if DefaultMaxHeaderBytes <= 0 {
		t.Fatalf("DefaultMaxHeaderBytes = %d", DefaultMaxHeaderBytes)
	}
}

// newTestServer creates an HTTP server bound to a random port with the
// given handler registered at the given pattern. It returns the server
// and the listener address (host:port).
func newTestServer(t *testing.T, pattern string, handler http.HandlerFunc) *Server {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

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
		t.Fatalf("New: %v", err)
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

	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Verify the server is accepting connections.
	addr := srv.Address()
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health status = %d, want 200", resp.StatusCode)
	}

	// Shutdown must complete without error (no double-close).
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// After shutdown, new connections must be refused.
	_, err = http.Get("http://" + addr + "/health")
	if err == nil {
		t.Fatal("expected connection refused after shutdown")
	}
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

	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

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

	if reqErr != nil {
		t.Fatalf("in-flight request error: %v", reqErr)
	}
	if respBody != "done" {
		t.Fatalf("in-flight response body = %q, want %q", respBody, "done")
	}

	// Shutdown must return nil (no double-close error).
	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestShutdown_NotStarted(t *testing.T) {
	srv := newTestServer(t, "", nil)

	// Shutdown on a server that was never started should be a no-op.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown on non-started server: %v", err)
	}
}

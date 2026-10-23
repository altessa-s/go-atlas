// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/transport/internal/timeouts"
)

func newLifecycleBase(t *testing.T) *BaseServer {
	t.Helper()
	s := NewBaseServer(WithAddress("test"), WithListener(&lifecycleListener{}), WithTimeouts(timeouts.Config{
		StartupVerification: 10 * time.Millisecond, ShutdownWarning: time.Second,
	}))
	t.Cleanup(func() {
		_ = s.GracefulShutdown(context.Background(), func(context.Context) error { return nil })
		if s.Listener() != nil {
			_ = s.Listener().Close()
		}
	})
	return s
}

func awaitDone(t *testing.T, s *BaseServer) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("Done did not close")
	}
}

func TestBaseServer_LateErrorAndRetry(t *testing.T) {
	s := newLifecycleBase(t)
	var logs bytes.Buffer
	s.options.logger = slog.New(slog.NewTextHandler(&logs, nil))
	done := s.Done()
	early := errors.New("early")
	if err := s.Start("test", func(_ net.Listener, ch chan<- error) { ch <- early }); err != early {
		t.Fatalf("Start = %v, want %v", err, early)
	}
	select {
	case <-done:
		t.Fatal("early error closed Done")
	default:
	}
	if s.Err() != nil {
		t.Fatalf("early Err = %v", s.Err())
	}
	var errCh chan<- error
	if err := s.Start("test", func(_ net.Listener, ch chan<- error) { errCh = ch }); err != nil {
		t.Fatal(err)
	}
	if s.Done() != done {
		t.Fatal("retry replaced Done")
	}
	late := errors.New("late")
	var readers sync.WaitGroup
	for range 8 {
		readers.Go(func() {
			for {
				_ = s.Err()
				select {
				case <-s.Done():
					return
				default:
				}
			}
		})
	}
	errCh <- late
	awaitDone(t, s)
	readers.Wait()
	if s.Err() != late {
		t.Fatalf("Err = %v, want original late error", s.Err())
	}
	if s.IsStarted() {
		t.Fatal("server still started after failure")
	}
	errCh <- errors.New("second")
	var calls atomic.Int32
	callback := func(context.Context) error { calls.Add(1); return nil }
	if err := s.GracefulShutdown(t.Context(), callback); err != nil {
		t.Fatal(err)
	}
	if err := s.GracefulShutdown(t.Context(), callback); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cleanup calls = %d", calls.Load())
	}
	if s.Err() != late {
		t.Fatal("shutdown replaced first error")
	}
	if count := strings.Count(logs.String(), "server serve error"); count != 1 {
		t.Fatalf("Serve error logged %d times, want 1", count)
	}
}

func TestBaseServer_ShutdownDone(t *testing.T) {
	for _, mode := range []string{"normal", "not-started", "canceled", "error"} {
		t.Run(mode, func(t *testing.T) {
			s := newLifecycleBase(t)
			var errCh chan<- error
			if mode != "not-started" {
				if err := s.Start("test", func(_ net.Listener, ch chan<- error) { errCh = ch }); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			want := errors.New("shutdown failed")
			err := s.GracefulShutdown(ctx, func(ctx context.Context) error {
				if mode == "not-started" {
					t.Error("unexpected shutdown callback")
				}
				if mode == "canceled" {
					<-ctx.Done()
					return ctx.Err()
				}
				// An error from Serve during shutdown must not become Err.
				errCh <- errors.New("listener closed during shutdown")
				deadline := time.Now().Add(time.Second)
				for len(errCh) != 0 && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if len(errCh) != 0 {
					return errors.New("watcher did not consume error")
				}
				if mode == "error" {
					return want
				}
				return nil
			})
			switch mode {
			case "canceled":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Shutdown = %v", err)
				}
			case "error":
				if err != want {
					t.Fatalf("Shutdown = %v", err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			awaitDone(t, s)
			if s.Err() != nil {
				t.Fatalf("Err = %v", s.Err())
			}
			select {
			case <-s.watchStop:
			default:
				t.Fatal("watcher not stopped")
			}
		})
	}
}

func TestBaseServer_ConcurrentStartShutdown(t *testing.T) {
	s := newLifecycleBase(t)
	entered, release := make(chan struct{}), make(chan struct{})
	startResult, stopResult := make(chan error, 1), make(chan error, 1)
	go func() { startResult <- s.Start("test", func(net.Listener, chan<- error) { close(entered); <-release }) }()
	<-entered
	go func() { stopResult <- s.GracefulShutdown(t.Context(), func(context.Context) error { return nil }) }()
	deadline := time.Now().Add(time.Second)
	for !s.shutdownWaiting.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	blocked := s.shutdownWaiting.Load()
	close(release)
	if !blocked {
		t.Fatal("Shutdown did not contend on lifecycle")
	}
	if err := <-startResult; err != nil {
		t.Fatal(err)
	}
	if err := <-stopResult; err != nil {
		t.Fatal(err)
	}
	awaitDone(t, s)
	if s.IsStarted() || s.Err() != nil {
		t.Fatalf("started=%v Err=%v", s.IsStarted(), s.Err())
	}
}

// lifecycleListener avoids opening a socket: BaseServer callbacks control serving.
type lifecycleListener struct{}

func (*lifecycleListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (*lifecycleListener) Close() error              { return nil }
func (*lifecycleListener) Addr() net.Addr            { return &net.TCPAddr{} }

func TestBaseServer_EarlyFailureRetryOwnedListener(t *testing.T) {
	for _, serveCloses := range []bool{false, true} {
		t.Run(fmt.Sprintf("serveCloses=%t", serveCloses), func(t *testing.T) {
			s := NewBaseServer(WithAddress("127.0.0.1:0"), WithTimeouts(timeouts.Config{
				StartupVerification: 10 * time.Millisecond, ShutdownWarning: time.Second,
			}))
			t.Cleanup(func() {
				_ = s.GracefulShutdown(context.Background(), func(context.Context) error {
					return s.Listener().Close()
				})
			})
			early := errors.New("early startup failure")
			var first net.Listener
			err := s.Start("test", func(ln net.Listener, ch chan<- error) {
				first = ln
				if serveCloses {
					_ = ln.Close()
				}
				ch <- early
			})
			if err != early {
				t.Fatalf("Start = %v, want %v", err, early)
			}
			if s.Listener() != nil {
				t.Fatal("early failure retained owned listener")
			}
			if _, err := first.Accept(); !errors.Is(err, net.ErrClosed) {
				t.Fatalf("first listener Accept = %v, want net.ErrClosed", err)
			}
			select {
			case <-s.Done():
				t.Fatal("early failure closed Done")
			default:
			}
			if err := s.Start("test", func(ln net.Listener, _ chan<- error) {
				if ln == first {
					t.Error("retry reused listener")
				}
			}); err != nil {
				t.Fatal(err)
			}
			conn, err := net.DialTimeout("tcp", s.Address(), time.Second)
			if err != nil {
				t.Fatalf("retry listener is not usable: %v", err)
			}
			_ = conn.Close()
			if s.Err() != nil {
				t.Fatalf("Err = %v", s.Err())
			}
		})
	}
}

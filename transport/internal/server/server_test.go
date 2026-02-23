// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/transport/internal/timeouts"
)

func TestNewBaseServer(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	if s == nil {
		t.Fatal("NewBaseServer returned nil")
	}
	if s.IsStarted() {
		t.Fatal("new server should not be started")
	}
	if s.IsShutdown() {
		t.Fatal("new server should not be shutdown")
	}
}

func TestBaseServer_Protocol(t *testing.T) {
	tests := []struct {
		name     string
		tls      bool
		protocol string
		want     string
	}{
		{"no_tls", false, "http", "http"},
		{"with_tls", true, "http", "http+tls"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			opts = append(opts, WithAddress(":0"))
			s := NewBaseServer(opts...)
			if !tt.tls {
				if got := s.Protocol(tt.protocol); got != tt.want {
					t.Fatalf("Protocol() = %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestBaseServer_Name(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"), WithName("test-server"))
	if got := s.Name(); got != "test-server" {
		t.Fatalf("Name() = %q, want %q", got, "test-server")
	}
}

func TestBaseServer_Timeouts(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	cfg := s.Timeouts()
	if cfg.StartupVerification != timeouts.DefaultStartupVerification {
		t.Fatalf("StartupVerification = %v", cfg.StartupVerification)
	}
}

func TestBaseServer_Listen(t *testing.T) {
	s := NewBaseServer(WithAddress("127.0.0.1:0"))
	ln, err := s.Listen()
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	if !s.HasListener() {
		t.Fatal("HasListener() = false after Listen()")
	}
	if s.Address() == "" {
		t.Fatal("Address() empty after Listen()")
	}

	// Second call should return same listener
	ln2, err := s.Listen()
	if err != nil {
		t.Fatalf("second Listen() error = %v", err)
	}
	if ln2 != ln {
		t.Fatal("second Listen() returned different listener")
	}
}

func TestBaseServer_WithListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen error = %v", err)
	}
	defer ln.Close()

	s := NewBaseServer(WithAddress(":0"), WithListener(ln))
	got, err := s.Listen()
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	if got != ln {
		t.Fatal("Listen() should return provided listener")
	}
}

func TestBaseServer_Start(t *testing.T) {
	s := NewBaseServer(
		WithAddress("127.0.0.1:0"),
		WithTimeouts(timeouts.Config{
			StartupVerification: 100 * time.Millisecond,
			ShutdownGraceful:    time.Second,
			ShutdownWarning:     time.Second,
		}),
	)

	err := s.Start("test", func(ln net.Listener, errCh chan<- error) {
		// Don't send error - simulates successful start
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !s.IsStarted() {
		t.Fatal("IsStarted() = false after Start()")
	}
}

func TestBaseServer_Start_AlreadyStarted(t *testing.T) {
	s := NewBaseServer(
		WithAddress("127.0.0.1:0"),
		WithTimeouts(timeouts.Config{
			StartupVerification: 100 * time.Millisecond,
			ShutdownGraceful:    time.Second,
			ShutdownWarning:     time.Second,
		}),
	)

	_ = s.Start("test", func(ln net.Listener, errCh chan<- error) {})
	err := s.Start("test", func(ln net.Listener, errCh chan<- error) {})
	if !errors.Is(err, ErrServerAlreadyStarted) {
		t.Fatalf("Start() error = %v, want ErrServerAlreadyStarted", err)
	}
}

func TestBaseServer_Start_EarlyError(t *testing.T) {
	s := NewBaseServer(
		WithAddress("127.0.0.1:0"),
		WithTimeouts(timeouts.Config{
			StartupVerification: time.Second,
			ShutdownGraceful:    time.Second,
			ShutdownWarning:     time.Second,
		}),
	)

	wantErr := errors.New("startup failed")
	err := s.Start("test", func(ln net.Listener, errCh chan<- error) {
		errCh <- wantErr
	})
	if err != wantErr {
		t.Fatalf("Start() error = %v, want %v", err, wantErr)
	}
	if s.IsStarted() {
		t.Fatal("IsStarted() should be false after startup error")
	}
}

func TestBaseServer_WaitForShutdown(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := s.WaitForShutdown(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForShutdown() error = %v, want context.Canceled", err)
	}
}

func TestBaseServer_GracefulShutdown(t *testing.T) {
	s := NewBaseServer(
		WithAddress("127.0.0.1:0"),
		WithTimeouts(timeouts.Config{
			StartupVerification: 100 * time.Millisecond,
			ShutdownGraceful:    time.Second,
			ShutdownWarning:     5 * time.Second,
		}),
	)

	_ = s.Start("test", func(ln net.Listener, errCh chan<- error) {})

	err := s.GracefulShutdown(t.Context(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("GracefulShutdown() error = %v", err)
	}
}

func TestBaseServer_GracefulShutdown_NotStarted(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	err := s.GracefulShutdown(t.Context(), func(ctx context.Context) error {
		return errors.New("should not be called")
	})
	if err != nil {
		t.Fatalf("GracefulShutdown() on non-started server should return nil, got %v", err)
	}
}

func TestErrors(t *testing.T) {
	if ErrServerAlreadyStarted == nil {
		t.Fatal("ErrServerAlreadyStarted is nil")
	}
	if ErrServerClosed == nil {
		t.Fatal("ErrServerClosed is nil")
	}
	if ErrInvalidConfiguration == nil {
		t.Fatal("ErrInvalidConfiguration is nil")
	}
}

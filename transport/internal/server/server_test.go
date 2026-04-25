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

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/timeouts"
)

func TestNewBaseServer(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	require.NotNil(t, s)
	require.False(t, s.IsStarted(), "new server should not be started")
	require.False(t, s.IsShutdown(), "new server should not be shutdown")
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
				got := s.Protocol(tt.protocol)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestBaseServer_Name(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"), WithName("test-server"))
	got := s.Name()
	require.Equal(t, "test-server", got)
}

func TestBaseServer_Timeouts(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	cfg := s.Timeouts()
	require.Equal(t, timeouts.DefaultStartupVerification, cfg.StartupVerification)
}

func TestBaseServer_Listen(t *testing.T) {
	s := NewBaseServer(WithAddress("127.0.0.1:0"))
	ln, err := s.Listen()
	require.NoError(t, err)
	defer ln.Close()

	require.True(t, s.HasListener(), "HasListener() = false after Listen()")
	require.NotEqual(t, "", s.Address())

	// Second call should return same listener
	ln2, err := s.Listen()
	require.NoError(t, err)
	require.Equal(t, ln, ln2)
}

func TestBaseServer_WithListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	s := NewBaseServer(WithAddress(":0"), WithListener(ln))
	got, err := s.Listen()
	require.NoError(t, err)
	require.Equal(t, ln, got)
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
	require.NoError(t, err)
	require.True(t, s.IsStarted(), "IsStarted() = false after Start()")
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
	require.True(t, errors.Is(err, ErrServerAlreadyStarted))
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
	require.Equal(t, wantErr, err)
	require.False(t, s.IsStarted(), "IsStarted() should be false after startup error")
}

func TestBaseServer_WaitForShutdown(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := s.WaitForShutdown(ctx)
	require.True(t, errors.Is(err, context.Canceled))
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
	require.NoError(t, err)

	require.True(t, s.IsShutdown(), "IsShutdown() should be true after GracefulShutdown")
	require.False(t, s.IsStarted(), "IsStarted() should be false after GracefulShutdown")
}

func TestBaseServer_GracefulShutdown_NotStarted(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	err := s.GracefulShutdown(t.Context(), func(ctx context.Context) error {
		return errors.New("should not be called")
	})
	require.NoError(t, err)
}

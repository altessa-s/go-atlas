// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"crypto/tls"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithLogger(t *testing.T) {
	logger := slog.Default()
	s := NewBaseServer(WithAddress(":0"), WithLogger(logger))
	require.False(t, s.Logger() != logger, "Logger() does not match provided logger")
}

func TestWithTlsConfig(t *testing.T) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}
	s := NewBaseServer(WithAddress(":0"), WithTlsConfig(cfg))
	require.False(t, s.TLSConfig() != cfg, "TLSConfig() does not match provided config")
	require.True(t, s.HasTLS(), "HasTLS() should be true")
}

func TestGetBase(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	require.False(t, s.GetBase() != s, "GetBase() should return itself")
}

func TestTLSConfig_Nil(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	require.False(t, s.TLSConfig() != nil, "TLSConfig() should be nil when not set")
	require.False(t, s.HasTLS(), "HasTLS() should be false")
}

func TestProtocol_WithTLS(t *testing.T) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}
	s := NewBaseServer(WithAddress(":0"), WithTlsConfig(cfg))
	got := s.Protocol("http")
	require.Equal(t, "http+tls", got)
}

func TestLogger_Default(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	require.False(t, s.Logger() == nil, "Logger() should not be nil by default")
}

func TestWithName_Pointer(t *testing.T) {
	name := "my-server"
	s := NewBaseServer(WithAddress(":0"), WithName(&name))
	got := s.Name()
	require.Equal(t, "my-server", got)
}

func TestWithAddress_Pointer(t *testing.T) {
	addr := ":8080"
	s := NewBaseServer(WithAddress(&addr))
	require.NotNil(t, s)
}

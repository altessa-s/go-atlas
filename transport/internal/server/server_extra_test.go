// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"crypto/tls"
	"log/slog"
	"testing"
)

func TestWithLogger(t *testing.T) {
	logger := slog.Default()
	s := NewBaseServer(WithAddress(":0"), WithLogger(logger))
	if s.Logger() != logger {
		t.Error("Logger() does not match provided logger")
	}
}

func TestWithTlsConfig(t *testing.T) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}
	s := NewBaseServer(WithAddress(":0"), WithTlsConfig(cfg))
	if s.TLSConfig() != cfg {
		t.Error("TLSConfig() does not match provided config")
	}
	if !s.HasTLS() {
		t.Error("HasTLS() should be true")
	}
}

func TestGetBase(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	if s.GetBase() != s {
		t.Error("GetBase() should return itself")
	}
}

func TestTLSConfig_Nil(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	if s.TLSConfig() != nil {
		t.Error("TLSConfig() should be nil when not set")
	}
	if s.HasTLS() {
		t.Error("HasTLS() should be false")
	}
}

func TestProtocol_WithTLS(t *testing.T) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}
	s := NewBaseServer(WithAddress(":0"), WithTlsConfig(cfg))
	if got := s.Protocol("http"); got != "http+tls" {
		t.Errorf("Protocol() = %q, want %q", got, "http+tls")
	}
}

func TestLogger_Default(t *testing.T) {
	s := NewBaseServer(WithAddress(":0"))
	if s.Logger() == nil {
		t.Error("Logger() should not be nil by default")
	}
}

func TestWithName_Pointer(t *testing.T) {
	name := "my-server"
	s := NewBaseServer(WithAddress(":0"), WithName(&name))
	if got := s.Name(); got != "my-server" {
		t.Errorf("Name() = %q, want %q", got, "my-server")
	}
}

func TestWithAddress_Pointer(t *testing.T) {
	addr := ":8080"
	s := NewBaseServer(WithAddress(&addr))
	if s == nil {
		t.Fatal("NewBaseServer with *string address returned nil")
	}
}

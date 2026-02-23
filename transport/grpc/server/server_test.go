// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"testing"
)

func TestNew(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if srv == nil {
		t.Fatal("server should not be nil")
	}
}

func TestNew_WithReflection(t *testing.T) {
	srv, err := New(WithReflection())
	if err != nil {
		t.Fatal(err)
	}
	if !srv.reflection {
		t.Fatal("reflection should be enabled")
	}
}

func TestServer_RegisterHandlers_Nil(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	srv.RegisterHandlers(nil, nil)
	if len(srv.handlers) != 0 {
		t.Fatalf("handlers = %d, want 0 (nil filtered)", len(srv.handlers))
	}
}

func TestServer_RegisterInterceptors_Empty(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.RegisterInterceptors(); err != nil {
		t.Fatal(err)
	}
}

func TestServer_IsStarted_Default(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if srv.IsStarted() {
		t.Fatal("should not be started")
	}
}

func TestServer_IsStopped_Default(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if srv.IsStopped() {
		t.Fatal("should not be stopped initially")
	}
}

func TestDefaultTimeoutConfig(t *testing.T) {
	cfg := DefaultTimeoutConfig()
	if cfg.StartupVerification <= 0 {
		t.Fatal("StartupVerification should be > 0")
	}
}

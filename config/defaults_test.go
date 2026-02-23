// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import "testing"

func TestDefaultAuth(t *testing.T) {
	cfg := DefaultAuth()
	if cfg.OIDC == nil {
		t.Fatal("expected non-nil OIDC")
	}
	if cfg.OPA == nil {
		t.Fatal("expected non-nil OPA")
	}
}

func TestDefaultGrpc(t *testing.T) {
	cfg := DefaultGrpc()
	if cfg.ListenAddress == "" {
		t.Fatal("expected non-empty ListenAddress")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}
}

func TestDefaultHttp(t *testing.T) {
	cfg := DefaultHttp()
	if cfg.ListenAddress == "" {
		t.Fatal("expected non-empty ListenAddress")
	}
	if cfg.MaxRequestPayloadSize == 0 {
		t.Fatal("expected non-zero MaxRequestPayloadSize")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}
}

func TestDefaultOIDC(t *testing.T) {
	cfg := DefaultOIDC()
	if cfg.ClockSkew == 0 {
		t.Fatal("expected non-zero ClockSkew")
	}
}

func TestDefaultRedis(t *testing.T) {
	cfg := DefaultRedis()
	if len(cfg.Hosts) == 0 {
		t.Fatal("expected non-empty Hosts")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}
}

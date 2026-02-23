// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsvault_test

import (
	"context"
	"testing"
	"time"

	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	tlsvault "github.com/altessa-s/go-atlas/security/tlsutils/providers/vault"
)

func TestNew_NoOptions(t *testing.T) {
	// New with no options creates a provider with zero-value defaults
	v, err := tlsvault.New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if v == nil {
		t.Fatal("New() returned nil")
	}
	defer v.Close(t.Context())
}

func TestNew_WithEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"valid https", "https://vault.example.com:8200", false},
		{"valid http", "http://localhost:8200", false},
		{"invalid url", "://invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tlsvault.New(
				tlsvault.WithEndpoint(tt.endpoint),
				tlsvault.WithStaticToken("s.token"),
				tlsvault.WithRole("test-role"),
				tlsvault.WithCommonName("test.example.com"),
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVault_Type(t *testing.T) {
	v, err := tlsvault.New(
		tlsvault.WithEndpoint("https://vault.example.com"),
		tlsvault.WithStaticToken("s.token"),
		tlsvault.WithRole("test-role"),
		tlsvault.WithCommonName("test.example.com"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer v.Close(t.Context())

	if got := v.Type(); got != tlsproviders.ProviderTypeVault {
		t.Errorf("Type() = %v, want %v", got, tlsproviders.ProviderTypeVault)
	}
}

func TestVault_TLSConfig(t *testing.T) {
	v, err := tlsvault.New(
		tlsvault.WithEndpoint("https://vault.example.com"),
		tlsvault.WithStaticToken("s.token"),
		tlsvault.WithRole("test-role"),
		tlsvault.WithCommonName("test.example.com"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer v.Close(t.Context())

	cfg, err := v.TLSConfig()
	if err != nil {
		t.Fatalf("TLSConfig() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("TLSConfig() returned nil")
	}
	if cfg.GetCertificate == nil {
		t.Error("TLSConfig() should set GetCertificate")
	}
	if cfg.GetClientCertificate == nil {
		t.Error("TLSConfig() should set GetClientCertificate")
	}
}

func TestVault_Close(t *testing.T) {
	v, err := tlsvault.New(
		tlsvault.WithEndpoint("https://vault.example.com"),
		tlsvault.WithStaticToken("s.token"),
		tlsvault.WithRole("test-role"),
		tlsvault.WithCommonName("test.example.com"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if err := v.Close(ctx); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestRSAGenerator_Generate(t *testing.T) {
	gen := tlsvault.NewRSAGenerator()
	key, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if key == nil {
		t.Fatal("Generate() returned nil key")
	}

	// Second call should return same key
	key2, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() second call error = %v", err)
	}
	if key != key2 {
		t.Error("Generate() should return cached key on subsequent calls")
	}
}

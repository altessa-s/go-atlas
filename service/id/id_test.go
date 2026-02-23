// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatic_ID(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"non-empty", "my-service-id"},
		{"empty", ""},
		{"uuid-like", "550e8400-e29b-41d4-a716-446655440000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewStatic(tt.id)
			if got := p.ID(); got != tt.id {
				t.Errorf("ID()=%q, want %q", got, tt.id)
			}
			// Idempotent
			if got := p.ID(); got != tt.id {
				t.Errorf("second ID()=%q, want %q", got, tt.id)
			}
		})
	}
}

func TestNewWithProvider_NilProvider(t *testing.T) {
	_, err := NewWithProvider(nil)
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestNewWithProvider_StaticProvider(t *testing.T) {
	s, err := NewWithProvider(NewStatic("test-id"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := s.ID(); got != "test-id" {
		t.Errorf("ID()=%q, want %q", got, "test-id")
	}
}

func TestNewStaticProvider(t *testing.T) {
	s, err := NewStaticProvider("abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := s.ID(); got != "abc" {
		t.Errorf("ID()=%q, want %q", got, "abc")
	}
}

func TestService_ID_NilProvider(t *testing.T) {
	s := &Service{}
	if got := s.ID(); got != "" {
		t.Errorf("ID()=%q, want empty for nil provider", got)
	}
}

func TestNewEnv_InvalidEnv(t *testing.T) {
	_, err := NewEnv("NONEXISTENT_ENV_VAR_FOR_TEST_12345")
	if err == nil {
		t.Fatal("expected error for nonexistent env var")
	}
}

func TestNewEnv_ValidEnv(t *testing.T) {
	const key = "TEST_SERVICE_ID_XYZ_12345"
	t.Setenv(key, "env-service-id")

	p, err := NewEnv(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := p.ID(); got != "env-service-id" {
		t.Errorf("ID()=%q, want %q", got, "env-service-id")
	}
}

func TestNewFile_EmptyPath(t *testing.T) {
	_, err := NewFile("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestNewFile_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	if err := os.WriteFile(path, []byte("EXISTING_ID_VALUE"), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := NewFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := p.ID(); got != "EXISTING_ID_VALUE" {
		t.Errorf("ID()=%q, want %q", got, "EXISTING_ID_VALUE")
	}
}

func TestNew_EnvFallbackToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	// No env set, should fall back to file
	s, err := New(path, "NONEXISTENT_ENV_FOR_TEST_12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id := s.ID()
	if id == "" {
		t.Fatal("expected non-empty ID from file provider")
	}
	if len(id) != 32 {
		t.Errorf("len(id)=%d, want 32", len(id))
	}
}

func TestNew_EnvTakesPriority(t *testing.T) {
	const key = "TEST_SERVICE_ID_PRIORITY_12345"
	t.Setenv(key, "from-env")

	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	s, err := New(path, key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := s.ID(); got != "from-env" {
		t.Errorf("ID()=%q, want %q", got, "from-env")
	}
}

func TestNewWithEnvProvider(t *testing.T) {
	const key = "TEST_ENV_PROVIDER_12345"
	t.Setenv(key, "env-val")

	s, err := NewWithEnvProvider(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := s.ID(); got != "env-val" {
		t.Errorf("ID()=%q, want %q", got, "env-val")
	}
}

func TestNewWithFileProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	s, err := NewWithFileProvider(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id := s.ID()
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
}

func TestMustNew_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid inputs")
		}
	}()
	MustNew("") // empty file path with no env should fail
}

func TestMustNewWithProvider_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil provider")
		}
	}()
	MustNewWithProvider(nil)
}

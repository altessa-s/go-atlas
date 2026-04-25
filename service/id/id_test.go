// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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
			require.Equal(t, tt.id, p.ID())
			// Idempotent
			require.Equal(t, tt.id, p.ID())
		})
	}
}

func TestNewWithProvider_NilProvider(t *testing.T) {
	_, err := NewWithProvider(nil)
	require.Error(t, err)
}

func TestNewWithProvider_StaticProvider(t *testing.T) {
	s, err := NewWithProvider(NewStatic("test-id"))
	require.NoError(t, err)
	require.Equal(t, "test-id", s.ID())
}

func TestNewStaticProvider(t *testing.T) {
	s, err := NewStaticProvider("abc")
	require.NoError(t, err)
	require.Equal(t, "abc", s.ID())
}

func TestService_ID_NilProvider(t *testing.T) {
	s := &Service{}
	require.Empty(t, s.ID(), "ID should be empty for nil provider")
}

func TestNewEnv_InvalidEnv(t *testing.T) {
	_, err := NewEnv("NONEXISTENT_ENV_VAR_FOR_TEST_12345")
	require.Error(t, err)
}

func TestNewEnv_ValidEnv(t *testing.T) {
	const key = "TEST_SERVICE_ID_XYZ_12345"
	t.Setenv(key, "env-service-id")

	p, err := NewEnv(key)
	require.NoError(t, err)
	require.Equal(t, "env-service-id", p.ID())
}

func TestNewFile_EmptyPath(t *testing.T) {
	_, err := NewFile("")
	require.Error(t, err)
}

func TestNewFile_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	require.NoError(t, os.WriteFile(path, []byte("EXISTING_ID_VALUE"), 0o600))

	p, err := NewFile(path)
	require.NoError(t, err)
	require.Equal(t, "EXISTING_ID_VALUE", p.ID())
}

func TestNew_EnvFallbackToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	// No env set, should fall back to file
	s, err := New(path, "NONEXISTENT_ENV_FOR_TEST_12345")
	require.NoError(t, err)
	id := s.ID()
	require.NotEmpty(t, id, "expected non-empty ID from file provider")
	require.Len(t, id, 32)
}

func TestNew_EnvTakesPriority(t *testing.T) {
	const key = "TEST_SERVICE_ID_PRIORITY_12345"
	t.Setenv(key, "from-env")

	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	s, err := New(path, key)
	require.NoError(t, err)
	require.Equal(t, "from-env", s.ID())
}

func TestNewWithEnvProvider(t *testing.T) {
	const key = "TEST_ENV_PROVIDER_12345"
	t.Setenv(key, "env-val")

	s, err := NewWithEnvProvider(key)
	require.NoError(t, err)
	require.Equal(t, "env-val", s.ID())
}

func TestNewWithFileProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.id")

	s, err := NewWithFileProvider(path)
	require.NoError(t, err)
	require.NotEmpty(t, s.ID())
}

func TestMustNew_Panics(t *testing.T) {
	defer func() {
		require.NotNil(t, recover(), "expected panic for invalid inputs")
	}()
	MustNew("") // empty file path with no env should fail
}

func TestMustNewWithProvider_Panics(t *testing.T) {
	defer func() {
		require.NotNil(t, recover(), "expected panic for nil provider")
	}()
	MustNewWithProvider(nil)
}

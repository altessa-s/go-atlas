// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmslocal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/mongo/kms"

	kmslocal "github.com/altessa-s/go-atlas/data/mongo/kms/local"
)

func validKey96() string {
	return string(make([]byte, 96))
}

func TestNew_WithMasterKey(t *testing.T) {
	p, err := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestNew_NoKey(t *testing.T) {
	_, err := kmslocal.New()
	require.Error(t, err)
}

func TestNew_InvalidKeyLength(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"too short", string(make([]byte, 32))},
		{"too long", string(make([]byte, 128))},
		{"one byte", "x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := kmslocal.New(kmslocal.WithMasterKey(tt.key))
			require.Error(t, err)
		})
	}
}

func TestNew_WithMasterKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "masterkey.bin")
	require.NoError(t, os.WriteFile(path, make([]byte, 96), 0600))

	p, err := kmslocal.New(kmslocal.WithMasterKeyFile(path))
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestNew_WithMasterKeyFile_NotFound(t *testing.T) {
	_, err := kmslocal.New(kmslocal.WithMasterKeyFile("/nonexistent/path"))
	require.Error(t, err)
}

func TestLocal_Name(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	require.Equal(t, "local", p.Name())
}

func TestLocal_Credentials(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	creds := p.Credentials()

	localCreds, ok := creds["local"]
	require.True(t, ok, "Credentials() missing 'local' key")
	require.NotNil(t, localCreds[kmslocal.MasterKey])
}

func TestLocal_Credentials_Cached(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	c1 := p.Credentials()
	c2 := p.Credentials()
	require.NotNil(t, c1["local"][kmslocal.MasterKey])
	require.NotNil(t, c2["local"][kmslocal.MasterKey])
}

func TestLocal_MasterKey_Nil(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	require.Nil(t, p.MasterKey())
}

func TestLocal_TLSConfig_Nil(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	require.Nil(t, p.TLSConfig())
}

func TestLocal_Clear(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	p.Clear()
	creds := p.Credentials()
	require.Len(t, creds["local"], 0)
}

func TestLocal_ImplementsProvider(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	var _ kms.Provider = p
}

func TestLocal_HasMasterKey_False(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	require.False(t, kms.HasMasterKey(p))
}

func TestLocal_HasCustomTLS_False(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	require.False(t, kms.HasCustomTLS(p))
}

func TestLocal_IsFullyFeatured_False(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	require.False(t, kms.IsFullyFeatured(p))
}

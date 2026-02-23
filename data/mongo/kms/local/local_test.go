// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmslocal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altessa-s/go-atlas/data/mongo/kms"

	kmslocal "github.com/altessa-s/go-atlas/data/mongo/kms/local"
)

func validKey96() string {
	return string(make([]byte, 96))
}

func TestNew_WithMasterKey(t *testing.T) {
	p, err := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if p == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_NoKey(t *testing.T) {
	_, err := kmslocal.New()
	if err == nil {
		t.Error("New() without key should return error")
	}
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
			if err == nil {
				t.Error("New() with invalid key length should return error")
			}
		})
	}
}

func TestNew_WithMasterKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "masterkey.bin")
	if err := os.WriteFile(path, make([]byte, 96), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	p, err := kmslocal.New(kmslocal.WithMasterKeyFile(path))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if p == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithMasterKeyFile_NotFound(t *testing.T) {
	_, err := kmslocal.New(kmslocal.WithMasterKeyFile("/nonexistent/path"))
	if err == nil {
		t.Error("New() with nonexistent file should return error")
	}
}

func TestLocal_Name(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	if p.Name() != "local" {
		t.Errorf("Name() = %q, want %q", p.Name(), "local")
	}
}

func TestLocal_Credentials(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	creds := p.Credentials()

	localCreds, ok := creds["local"]
	if !ok {
		t.Fatal("Credentials() missing 'local' key")
	}
	if localCreds[kmslocal.MasterKey] == nil {
		t.Error("MasterKey should not be nil")
	}
}

func TestLocal_Credentials_Cached(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	c1 := p.Credentials()
	c2 := p.Credentials()
	if c1["local"][kmslocal.MasterKey] == nil || c2["local"][kmslocal.MasterKey] == nil {
		t.Error("Cached credentials should be consistent")
	}
}

func TestLocal_MasterKey_Nil(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	if p.MasterKey() != nil {
		t.Error("MasterKey() should return nil for local provider")
	}
}

func TestLocal_TLSConfig_Nil(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	if p.TLSConfig() != nil {
		t.Error("TLSConfig() should return nil for local provider")
	}
}

func TestLocal_Clear(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	p.Clear()
	creds := p.Credentials()
	if len(creds["local"]) != 0 {
		t.Errorf("Credentials() after Clear() should be empty, got %v", creds["local"])
	}
}

func TestLocal_ImplementsProvider(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	var _ kms.Provider = p
}

func TestLocal_HasMasterKey_False(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	if kms.HasMasterKey(p) {
		t.Error("Local provider should not have master key")
	}
}

func TestLocal_HasCustomTLS_False(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	if kms.HasCustomTLS(p) {
		t.Error("Local provider should not have custom TLS")
	}
}

func TestLocal_IsFullyFeatured_False(t *testing.T) {
	p, _ := kmslocal.New(kmslocal.WithMasterKey(validKey96()))
	if kms.IsFullyFeatured(p) {
		t.Error("Local provider should not be fully featured")
	}
}

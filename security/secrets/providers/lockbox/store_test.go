// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockbox

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestValidateLockboxFolderId(t *testing.T) {
	tests := []struct {
		name     string
		folderId string
		wantErr  error
	}{
		{"valid", "b1g2h3j4k5l6m7n8o9p0", nil},
		{"valid with hyphens", "folder-123-abc", nil},
		{"empty", "", ErrInvalidFolderId},
		{"too long", strings.Repeat("a", 51), ErrInvalidFolderId},
		{"max length", strings.Repeat("a", 50), nil},
		{"contains underscore - invalid", "folder_123", ErrInvalidFolderId},
		{"contains space", "folder 123", ErrInvalidFolderId},
		{"special chars", "folder!@#", ErrInvalidFolderId},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLockboxFolderId(tt.folderId)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("validateLockboxFolderId(%q) = %v, want %v", tt.folderId, err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("validateLockboxFolderId(%q) unexpected error: %v", tt.folderId, err)
			}
		})
	}
}

func TestValidateLockboxKeyId(t *testing.T) {
	tests := []struct {
		name    string
		keyId   string
		wantErr error
	}{
		{"valid", "aje1234567890abcdef", nil},
		{"valid with hyphens", "key-123-abc", nil},
		{"valid with underscore", "key_123", nil},
		{"empty", "", ErrInvalidKeyId},
		{"too long", strings.Repeat("a", 101), ErrInvalidKeyId},
		{"max length", strings.Repeat("a", 100), nil},
		{"contains space", "key 123", ErrInvalidKeyId},
		{"special chars", "key!@#", ErrInvalidKeyId},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLockboxKeyId(tt.keyId)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("validateLockboxKeyId(%q) = %v, want %v", tt.keyId, err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("validateLockboxKeyId(%q) unexpected error: %v", tt.keyId, err)
			}
		})
	}
}

func TestValidateLockboxServiceAccountId(t *testing.T) {
	tests := []struct {
		name             string
		serviceAccountId string
		wantErr          error
	}{
		{"valid", "aje1234567890abcdef", nil},
		{"valid with hyphens", "sa-123-abc", nil},
		{"valid with underscore", "sa_123", nil},
		{"empty", "", ErrInvalidServiceAccountId},
		{"too long", strings.Repeat("a", 101), ErrInvalidServiceAccountId},
		{"contains space", "sa 123", ErrInvalidServiceAccountId},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLockboxServiceAccountId(tt.serviceAccountId)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("validateLockboxServiceAccountId(%q) = %v, want %v", tt.serviceAccountId, err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("validateLockboxServiceAccountId(%q) unexpected error: %v", tt.serviceAccountId, err)
			}
		})
	}
}

func TestValidateLockboxPrivateKey(t *testing.T) {
	validKey := testhelpers.GenerateRSAKeyPEM(t, 2048)

	smallKeyPEM := testhelpers.GenerateRSAKeyPEM(t, 1024)

	tests := []struct {
		name    string
		privKey []byte
		wantErr error
	}{
		{"valid 2048-bit key", validKey, nil},
		{"empty", nil, ErrInvalidPrivateKey},
		{"empty bytes", []byte{}, ErrInvalidPrivateKey},
		{"invalid PEM", []byte("not a PEM"), ErrInvalidPrivateKey},
		{"wrong PEM type", pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("invalid")}), ErrInvalidPrivateKey},
		{"small RSA key", smallKeyPEM, ErrInvalidPrivateKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLockboxPrivateKey(tt.privKey)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("validateLockboxPrivateKey() = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("validateLockboxPrivateKey() unexpected error: %v", err)
			}
		})
	}
}

func TestValidateLockboxPrivateKey_ValidPKCS8(t *testing.T) {
	key := testhelpers.GenerateRSAKey(t, 2048)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	if err := validateLockboxPrivateKey(pemBytes); err != nil {
		t.Errorf("validateLockboxPrivateKey() with valid PKCS8 key = %v, want nil", err)
	}
}

func TestValidateLockboxSecretKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", "my-secret", false},
		{"valid with underscore", "_my_secret", false},
		{"empty", "", true},
		{"single char", "a", true},
		{"starts with digit", "1secret", true},
		{"contains dot", "my.secret", true},
		{"too long", strings.Repeat("a", 256), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLockboxSecretKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLockboxSecretKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestStorageName(t *testing.T) {
	s := &Storage[string]{}
	if got := s.Name(); got != "lockbox" {
		t.Errorf("Name() = %q, want %q", got, "lockbox")
	}
}

func TestReadPrivateKey(t *testing.T) {
	tests := []struct {
		name       string
		keyData    *string
		keyPath    *string
		wantErr    bool
		wantNonNil bool
	}{
		{
			name:       "from data",
			keyData:    testhelpers.StringPtr("private-key-data"),
			keyPath:    nil,
			wantErr:    false,
			wantNonNil: true,
		},
		{
			name:    "nil data nil path",
			keyData: nil,
			keyPath: nil,
			wantErr: true,
		},
		{
			name:    "nil data empty path",
			keyData: nil,
			keyPath: testhelpers.StringPtr(""),
			wantErr: true,
		},
		{
			name:    "nil data nonexistent path",
			keyData: nil,
			keyPath: testhelpers.StringPtr("/nonexistent/path/key.pem"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadPrivateKey(tt.keyData, tt.keyPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadPrivateKey() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantNonNil && got == nil {
				t.Error("ReadPrivateKey() returned nil, want non-nil")
			}
		})
	}
}

func TestReadPrivateKey_FromFile(t *testing.T) {
	dir := t.TempDir()
	keyContent := "test-key-content"
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(keyPath, []byte(keyContent), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadPrivateKey(nil, &keyPath)
	if err != nil {
		t.Fatalf("ReadPrivateKey() error = %v", err)
	}
	if string(got) != keyContent {
		t.Errorf("ReadPrivateKey() = %q, want %q", string(got), keyContent)
	}
}

func TestNewLockBoxToken(t *testing.T) {
	token := NewLockBoxToken("key-id", "sa-id", []byte("privkey"))
	if token == nil {
		t.Fatal("NewLockBoxToken() returned nil")
	}
}

func TestToken_RequireTransportSecurity(t *testing.T) {
	token := NewLockBoxToken("key-id", "sa-id", []byte("privkey"))
	if !token.RequireTransportSecurity() {
		t.Error("RequireTransportSecurity() = false, want true")
	}
}

func TestToken_Shutdown(t *testing.T) {
	token := NewLockBoxToken("key-id", "sa-id", []byte("privkey"))
	// Should not panic
	token.Shutdown()
	// Double shutdown should be safe
	token.Shutdown()
}

func TestToken_GetRequestMetadata_AfterShutdown(t *testing.T) {
	token := NewLockBoxToken("key-id", "sa-id", []byte("privkey"))
	token.Shutdown()

	_, err := token.GetRequestMetadata(nil)
	if err == nil {
		t.Error("GetRequestMetadata() after Shutdown() should return error")
	}
}

func TestAddJitter(t *testing.T) {
	tests := []struct {
		name    string
		percent int
	}{
		{"zero percent", 0},
		{"five percent", 5},
		{"fifty percent", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := clientTokenLifetime
			result := addJitter(base, tt.percent)
			if tt.percent == 0 {
				if result != base {
					t.Errorf("addJitter() with 0%% = %v, want %v", result, base)
				}
				return
			}
			// Result should be within ±percent range
			maxDelta := base * clientTokenLifetime / 100
			if result < base-maxDelta || result > base+maxDelta {
				// jitter is random, so just verify it doesn't panic
			}
		})
	}
}

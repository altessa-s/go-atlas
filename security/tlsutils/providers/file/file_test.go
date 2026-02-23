// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsfile_test

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	tlsfile "github.com/altessa-s/go-atlas/security/tlsutils/providers/file"
)

func TestNewWithCertAndKey_EmptyCertFile(t *testing.T) {
	_, err := tlsfile.NewWithCertAndKey("", "key.pem", "")
	if err == nil {
		t.Error("NewWithCertAndKey with empty cert file should return error")
	}
}

func TestNewWithCertAndKey_EmptyKeyFile(t *testing.T) {
	_, err := tlsfile.NewWithCertAndKey("cert.pem", "", "")
	if err == nil {
		t.Error("NewWithCertAndKey with empty key file should return error")
	}
}

func TestNewWithCertAndKey_NonexistentFiles(t *testing.T) {
	_, err := tlsfile.NewWithCertAndKey("/nonexistent/cert.pem", "/nonexistent/key.pem", "")
	if err == nil {
		t.Error("NewWithCertAndKey with nonexistent files should return error")
	}
}

func TestNewWithCertAndKey_ValidCert(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	if err != nil {
		t.Fatalf("NewWithCertAndKey() error = %v", err)
	}
	defer f.Close(t.Context())

	if f.Type() != tlsproviders.ProviderTypeFile {
		t.Errorf("Type() = %v, want %v", f.Type(), tlsproviders.ProviderTypeFile)
	}
}

func TestFile_TLSConfig(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	if err != nil {
		t.Fatalf("NewWithCertAndKey() error = %v", err)
	}
	defer f.Close(t.Context())

	config, err := f.TLSConfig()
	if err != nil {
		t.Fatalf("TLSConfig() error = %v", err)
	}
	if config == nil {
		t.Fatal("TLSConfig() returned nil")
	}
	if len(config.Certificates) == 0 {
		t.Error("TLSConfig() has no certificates")
	}
}

func TestFile_TLSConfig_ReturnsCopy(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	if err != nil {
		t.Fatalf("NewWithCertAndKey() error = %v", err)
	}
	defer f.Close(t.Context())

	cfg1, _ := f.TLSConfig()
	cfg2, _ := f.TLSConfig()

	// Modifying one should not affect the other
	cfg1.ServerName = "modified"
	if cfg2.ServerName == "modified" {
		t.Error("TLSConfig() should return independent copies")
	}
}

func TestFile_Close(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	if err != nil {
		t.Fatalf("NewWithCertAndKey() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if err := f.Close(ctx); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestFile_Type(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	if err != nil {
		t.Fatalf("NewWithCertAndKey() error = %v", err)
	}
	defer f.Close(t.Context())

	if got := f.Type(); got != tlsproviders.ProviderTypeFile {
		t.Errorf("Type() = %v, want %v", got, tlsproviders.ProviderTypeFile)
	}
}

func TestNewWithCertAndKey_InvalidParams(t *testing.T) {
	tests := []struct {
		name     string
		certFile string
		keyFile  string
		wantErr  bool
	}{
		{"both empty", "", "", true},
		{"cert empty", "", "key.pem", true},
		{"key empty", "cert.pem", "", true},
		{"nonexistent", "/tmp/nonexistent.pem", "/tmp/nonexistent.key", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tlsfile.NewWithCertAndKey(tt.certFile, tt.keyFile, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("NewWithCertAndKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

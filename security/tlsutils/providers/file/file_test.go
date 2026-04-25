// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsfile_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	tlsfile "github.com/altessa-s/go-atlas/security/tlsutils/providers/file"
)

func TestNewWithCertAndKey_EmptyCertFile(t *testing.T) {
	_, err := tlsfile.NewWithCertAndKey("", "key.pem", "")
	require.Error(t, err)
}

func TestNewWithCertAndKey_EmptyKeyFile(t *testing.T) {
	_, err := tlsfile.NewWithCertAndKey("cert.pem", "", "")
	require.Error(t, err)
}

func TestNewWithCertAndKey_NonexistentFiles(t *testing.T) {
	_, err := tlsfile.NewWithCertAndKey("/nonexistent/cert.pem", "/nonexistent/key.pem", "")
	require.Error(t, err)
}

func TestNewWithCertAndKey_ValidCert(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	require.NoError(t, err)
	defer f.Close(t.Context())

	require.Equal(t, tlsproviders.ProviderTypeFile, f.Type())
}

func TestFile_TLSConfig(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	require.NoError(t, err)
	defer f.Close(t.Context())

	config, err := f.TLSConfig()
	require.NoError(t, err)
	require.NotNil(t, config)
	require.NotEmpty(t, config.Certificates)
}

func TestFile_TLSConfig_ReturnsCopy(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	require.NoError(t, err)
	defer f.Close(t.Context())

	cfg1, _ := f.TLSConfig()
	cfg2, _ := f.TLSConfig()

	// Modifying one should not affect the other
	cfg1.ServerName = "modified"
	require.NotEqual(t, "modified", cfg2.ServerName)
}

func TestFile_Close(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	require.NoError(t, f.Close(ctx))
}

func TestFile_Type(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	require.NoError(t, err)
	defer f.Close(t.Context())

	require.Equal(t, tlsproviders.ProviderTypeFile, f.Type())
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
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

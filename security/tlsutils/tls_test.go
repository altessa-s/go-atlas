// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsutils_test

import (
	"crypto/tls"
	"testing"

	"github.com/altessa-s/go-atlas/security/tlsutils"
)

func TestDefaultTLSConfig(t *testing.T) {
	config := tlsutils.DefaultTLSConfig()
	if config.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %v, want tls.VersionTLS12", config.MinVersion)
	}
	if len(config.CipherSuites) == 0 {
		t.Error("CipherSuites should not be empty")
	}
}

func TestDefaultClientTLSConfig(t *testing.T) {
	config := tlsutils.DefaultClientTLSConfig("example.com")
	if config.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %v, want tls.VersionTLS12", config.MinVersion)
	}
	if config.ServerName != "example.com" {
		t.Errorf("ServerName = %q, want %q", config.ServerName, "example.com")
	}
}

func TestCloneCertificateWithOCSPStaple(t *testing.T) {
	original := &tls.Certificate{
		Certificate:                 [][]byte{{0x01, 0x02}},
		PrivateKey:                  "test-key",
		SignedCertificateTimestamps: [][]byte{{0x04, 0x05}},
	}
	staple := []byte{0x06, 0x07, 0x08}

	cloned := tlsutils.CloneCertificateWithOCSPStaple(original, staple)

	if len(cloned.OCSPStaple) != len(staple) {
		t.Errorf("OCSPStaple length = %d, want %d", len(cloned.OCSPStaple), len(staple))
	}
	if len(cloned.Certificate) != len(original.Certificate) {
		t.Error("Certificate not preserved")
	}
	if cloned.PrivateKey != original.PrivateKey {
		t.Error("PrivateKey not preserved")
	}
}

func TestBuildCAPool_SystemOnly(t *testing.T) {
	pool, err := tlsutils.BuildCAPool(true)
	if err != nil {
		t.Skipf("system cert pool not available: %v", err)
	}
	if pool == nil {
		t.Error("expected non-nil pool")
	}
}

func TestBuildCAPool_InvalidFile(t *testing.T) {
	_, err := tlsutils.BuildCAPool(false, "/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadFromFile_InvalidPaths(t *testing.T) {
	_, err := tlsutils.LoadFromFile("/nonexistent/key", "/nonexistent/cert", "")
	if err == nil {
		t.Error("expected error for nonexistent paths")
	}
}

func TestLoadFromConcatenatedFile_InvalidPath(t *testing.T) {
	_, err := tlsutils.LoadFromConcatenatedFile("/nonexistent/file", "")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

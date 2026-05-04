// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxydial

import (
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"testing"
)

func BenchmarkTLSConfig_NilUser(b *testing.B) {
	u, _ := url.Parse("https://proxy.example.com:8443")
	for b.Loop() {
		_ = TLSConfig(u, nil)
	}
}

func BenchmarkTLSConfig_WithUserConfig(b *testing.B) {
	u, _ := url.Parse("https://proxy.example.com:8443")
	user := &tls.Config{RootCAs: x509.NewCertPool()}
	for b.Loop() {
		_ = TLSConfig(u, user)
	}
}

func BenchmarkBasicAuthHeader(b *testing.B) {
	auth := url.UserPassword("svc", "secret")
	for b.Loop() {
		_ = basicAuthHeader(auth)
	}
}

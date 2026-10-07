// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"testing"
	"time"
)

func BenchmarkNewOCSPStapler(b *testing.B) {
	for b.Loop() {
		_ = NewOCSPStapler()
	}
}

// BenchmarkGetOCSPStaple_CacheHit_Compression measures the handshake hot path:
// a valid cached response served with compression enabled.
func BenchmarkGetOCSPStaple_CacheHit_Compression(b *testing.B) {
	s := NewOCSPStapler(WithCompression())
	der := make([]byte, 1200) // typical DER OCSP response size
	for i := range der {
		der[i] = byte(i * 31)
	}
	cert := &tls.Certificate{Certificate: [][]byte{[]byte("leaf-der")}, Leaf: &x509.Certificate{}}
	s.cache[base64.StdEncoding.EncodeToString(cert.Certificate[0])] =
		&ocspCacheEntry{response: der, nextUpdate: time.Now().Add(24 * time.Hour)}
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.GetOCSPStaple(ctx, cert); err != nil {
			b.Fatal(err)
		}
	}
}

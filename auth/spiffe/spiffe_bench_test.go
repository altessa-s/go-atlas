// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spiffe_test

import (
	"crypto/x509"
	"net/url"
	"testing"

	"github.com/altessa-s/go-atlas/auth/spiffe"
)

func BenchmarkParseID(b *testing.B) {
	const raw = "spiffe://example.org/ns/default/sa/billing"
	b.ReportAllocs()
	for b.Loop() {
		if _, err := spiffe.ParseID(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIDFromCertificate(b *testing.B) {
	u, _ := url.Parse("spiffe://example.org/ns/default/sa/billing")
	cert := &x509.Certificate{URIs: []*url.URL{u}}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := spiffe.IDFromCertificate(cert); err != nil {
			b.Fatal(err)
		}
	}
}

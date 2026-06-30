// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"net/url"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/auth/mtls"
)

func BenchmarkAuthenticate(b *testing.B) {
	u, _ := url.Parse("spiffe://example.org/sa/billing")
	cert := &x509.Certificate{
		URIs:      []*url.URL{u},
		NotBefore: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	a := mtls.NewAuthenticator(
		mtls.WithValidator(mtls.ExpiryValidator(nil, time.Minute)),
		mtls.WithValidator(mtls.TrustDomainValidator("example.org")),
	)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := a.Authenticate(ctx, cert); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidators(b *testing.B) {
	u, _ := url.Parse("spiffe://example.org/sa/billing")
	cert := &x509.Certificate{
		URIs:           []*url.URL{u},
		Subject:        pkix.Name{CommonName: "billing"},
		Issuer:         pkix.Name{CommonName: "Acme Root CA"},
		AuthorityKeyId: []byte{1, 2, 3},
		DNSNames:       []string{"api.example.org"},
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		NotBefore:      time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:       time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	a := mtls.NewAuthenticator(
		mtls.WithValidator(mtls.SubjectValidator(pkix.Name{CommonName: "billing"})),
		mtls.WithValidator(mtls.IssuerValidator(pkix.Name{CommonName: "Acme Root CA"})),
		mtls.WithValidator(mtls.AuthorityKeyIDValidator([]byte{1, 2, 3})),
		mtls.WithValidator(mtls.DNSNameValidator("api.example.org")),
		mtls.WithValidator(mtls.EKUValidator(x509.ExtKeyUsageClientAuth)),
	)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := a.Authenticate(ctx, cert); err != nil {
			b.Fatal(err)
		}
	}
}

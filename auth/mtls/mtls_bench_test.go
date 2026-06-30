// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"crypto/x509"
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

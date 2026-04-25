// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsfile_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	tlsfile "github.com/altessa-s/go-atlas/security/tlsutils/providers/file"
)

func BenchmarkNewWithCertAndKey(b *testing.B) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(b)
	certPath, keyPath := testhelpers.WriteTempCertFiles(b, certPEM, keyPEM)

	b.ResetTimer()
	for b.Loop() {
		f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
		if err != nil {
			b.Fatal(err)
		}
		f.Close(b.Context())
	}
}

func BenchmarkTLSConfig(b *testing.B) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(b)
	certPath, keyPath := testhelpers.WriteTempCertFiles(b, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_, _ = f.TLSConfig()
	}
}

func BenchmarkType(b *testing.B) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(b)
	certPath, keyPath := testhelpers.WriteTempCertFiles(b, certPEM, keyPEM)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_ = f.Type()
	}
}

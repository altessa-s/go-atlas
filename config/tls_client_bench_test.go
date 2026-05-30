// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import "testing"

func BenchmarkTlsClientNormalize(b *testing.B) {
	c := TlsClient{
		Certificate: "/tmp/cert.pem",
		PrivateKey:  "/tmp/key.pem",
		CACerts:     []string{"/tmp/ca.pem"},
		SkipVerify:  false,
	}

	for b.Loop() {
		c.Normalize()
	}
}

func BenchmarkTlsClientNormalize_SkipVerifyMode(b *testing.B) {
	c := TlsClient{
		SkipVerifyMode: TLSSkipVerifyModeEnforce,
	}

	for b.Loop() {
		c.Normalize()
	}
}

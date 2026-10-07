// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsconfig_test

import (
	"testing"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
)

func BenchmarkTlsClientNormalize(b *testing.B) {
	c := tlsconfig.Client{
		Certificate: "/tmp/cert.pem",
		PrivateKey:  "/tmp/key.pem",
		CACerts:     []string{"/tmp/ca.pem"},
		SkipVerify:  false,
	}

	for b.Loop() {
		c.Normalize()
	}
}

func BenchmarkTlsClientNormalizeSkipVerifyMode(b *testing.B) {
	c := tlsconfig.Client{
		SkipVerifyMode: tlsconfig.SkipVerifyModeEnforce,
	}

	for b.Loop() {
		c.Normalize()
	}
}

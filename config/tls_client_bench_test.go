// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config"
)

func BenchmarkTlsClientNormalize(b *testing.B) {
	c := config.TlsClient{
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
	c := config.TlsClient{
		SkipVerifyMode: config.TLSSkipVerifyModeEnforce,
	}

	for b.Loop() {
		c.Normalize()
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/mtls/factory"
	"github.com/altessa-s/go-atlas/config"
)

func BenchmarkOptions(b *testing.B) {
	cfg := &config.MTLS{TrustDomains: []string{"example.org"}, CheckExpiry: true}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := factory.New(cfg).Options(); err != nil {
			b.Fatal(err)
		}
	}
}

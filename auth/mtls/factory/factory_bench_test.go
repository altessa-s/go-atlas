// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/mtls/factory"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

func BenchmarkOptions(b *testing.B) {
	cfg := &authconfig.MTLS{TrustDomains: []string{"example.org"}, CheckExpiry: true}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := factory.New(cfg).Options(); err != nil {
			b.Fatal(err)
		}
	}
}

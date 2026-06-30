// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spiffe_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/tlsutils/spiffe"
)

func BenchmarkMTLSServerConfig(b *testing.B) {
	p, err := spiffe.NewProvider(&fakeSource{}, anyAuthorizer())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = p.MTLSServerConfig()
	}
}

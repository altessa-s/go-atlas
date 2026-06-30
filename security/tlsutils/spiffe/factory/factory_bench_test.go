// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/security/tlsutils/spiffe/factory"
)

func BenchmarkAuthorizer(b *testing.B) {
	builder := factory.New(&config.SPIFFE{AllowedTrustDomains: []string{"example.org", "other.org"}})
	b.ReportAllocs()
	for b.Loop() {
		if _, err := builder.Authorizer(); err != nil {
			b.Fatal(err)
		}
	}
}

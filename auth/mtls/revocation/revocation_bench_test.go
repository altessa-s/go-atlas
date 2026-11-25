// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation_test

import (
	"crypto/x509"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ocsp"
)

// BenchmarkCheckCached measures the steady-state hot path: a cache hit, with no
// network round-trip (the responder is queried once to prime the cache).
func BenchmarkCheckCached(b *testing.B) {
	ca := makeCA(b)
	var hits atomic.Int32
	srv := responder(b, ca, ocsp.Good, &hits)
	leaf := makeLeaf(b, ca, 42, srv.URL)

	c := newChecker([]*x509.Certificate{ca.Cert})
	if err := c.Check(b.Context(), leaf); err != nil { // prime cache
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := c.Check(b.Context(), leaf); err != nil {
			b.Fatal(err)
		}
	}
}

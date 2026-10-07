// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/keyset"
)

func BenchmarkCodec_Issue(b *testing.B) {
	c, _ := keyset.New(key)
	payload := []byte("1700000000000|01JABCDEFGHJKMNPQRSTVWXYZ00")
	b.ReportAllocs()
	for b.Loop() {
		_, _ = c.Issue(payload, bindings)
	}
}

func BenchmarkCodec_Resolve(b *testing.B) {
	c, _ := keyset.New(key)
	token, _ := c.Issue([]byte("1700000000000|01JABCDEFGHJKMNPQRSTVWXYZ00"), bindings)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = c.Resolve(token, bindings)
	}
}

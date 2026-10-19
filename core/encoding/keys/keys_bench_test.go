// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keys_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/encoding/keys"
)

var (
	composeSink string
	digestSink  []byte
)

func BenchmarkCompose(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		composeSink = keys.Compose("transport", "POST:/api/v1/users", "550e8400-e29b-41d4-a716-446655440000")
	}
}

func BenchmarkDigest(b *testing.B) {
	method, body := []byte("/billing.v1.ProviderService/CreateProvider"), []byte(`{"name":"p","baseUrl":"https://example.com"}`)
	b.ReportAllocs()
	for b.Loop() {
		digestSink = keys.Digest(method, body)
	}
}

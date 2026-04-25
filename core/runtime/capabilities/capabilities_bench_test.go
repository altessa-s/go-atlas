// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package capabilities_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/capabilities"
)

func BenchmarkParseNameHit(b *testing.B) {
	var sink capabilities.Cap
	for b.Loop() {
		sink, _ = capabilities.ParseName("CAP_NET_BIND_SERVICE")
	}
	_ = sink
}

func BenchmarkParseNameMiss(b *testing.B) {
	var err error
	for b.Loop() {
		_, err = capabilities.ParseName("CAP_NOT_A_REAL_CAP")
	}
	_ = err
}

func BenchmarkCapString(b *testing.B) {
	c := capabilities.CAP_NET_BIND_SERVICE
	var sink string
	for b.Loop() {
		sink = c.String()
	}
	_ = sink
}

func BenchmarkSetsHas(b *testing.B) {
	s := capabilities.Sets{}.With(
		capabilities.CAP_NET_BIND_SERVICE,
		capabilities.Effective,
		capabilities.Permitted,
	)
	var sink bool
	for b.Loop() {
		sink = s.Has(capabilities.CAP_NET_BIND_SERVICE)
	}
	_ = sink
}

func BenchmarkSetsWith(b *testing.B) {
	var sink capabilities.Sets
	for b.Loop() {
		sink = capabilities.Sets{}.With(
			capabilities.CAP_NET_BIND_SERVICE,
			capabilities.Effective,
		)
	}
	_ = sink
}

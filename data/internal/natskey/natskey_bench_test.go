// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskey_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/internal/natskey"
)

func BenchmarkEncode(b *testing.B) {
	for _, bc := range []struct {
		name string
		key  string
	}{
		{name: "Unchanged", key: "tenant-1/orders.v2/0123456789abcdef"},
		{name: "Colons", key: "idk:POST:/orders:0123456789abcdef"},
		{name: "Unicode", key: "κλειδί-ταυτότητας"},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = natskey.Encode(bc.key)
			}
		})
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import (
	"fmt"
	"sync"
	"testing"
)

func BenchmarkGetOrCreate_Existing(b *testing.B) {
	var m sync.Map
	m.Store("key", "value")
	b.ResetTimer()
	for b.Loop() {
		GetOrCreate(&m, "key", func() string { return "new" })
	}
}

func BenchmarkGetOrCreate_New(b *testing.B) {
	var m sync.Map
	var i int
	b.ResetTimer()
	for b.Loop() {
		key := fmt.Sprintf("key-%d", i)
		GetOrCreate(&m, key, func() string { return "value" })
		i++
	}
}

func BenchmarkGetOrCreateWithCallback_Existing(b *testing.B) {
	var m sync.Map
	m.Store("key", "value")
	b.ResetTimer()
	for b.Loop() {
		GetOrCreateWithCallback(&m, "key",
			func() string { return "new" },
			func() {},
		)
	}
}

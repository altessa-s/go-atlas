// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"
	"time"
)

// BenchmarkMergeFetchBatches merges two full fetch batches of the default
// size, as a fetch with both pending and failed events does.
func BenchmarkMergeFetchBatches(b *testing.B) {
	const batch = 200
	base := time.Unix(1_700_000_000, 0).UTC()
	pending, failed := make([]int, batch), make([]int, batch)
	for i := range batch {
		pending[i], failed[i] = 2*i, 2*i+1
	}
	a, c := fetchEvents("p", base, pending...), fetchEvents("f", base, failed...)
	for b.Loop() {
		mergeFetchBatches(a, c, batch)
	}
}

func BenchmarkDefaultOptions(b *testing.B) {
	for b.Loop() {
		defaultOptions()
	}
}

func BenchmarkNewOptions(b *testing.B) {
	for b.Loop() {
		newOptions(WithCollectionName("events"))
	}
}

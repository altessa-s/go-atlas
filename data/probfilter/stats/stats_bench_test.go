// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package stats_test

import (
	"encoding/json"
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter/stats"
)

func BenchmarkFilterStats_Marshal(b *testing.B) {
	fs := &stats.FilterStats{
		Capacity:          100000,
		ItemCount:         5000,
		FillRatio:         0.05,
		FalsePositiveRate: 0.01,
		StorageType:       "memory",
	}
	for b.Loop() {
		_, _ = json.Marshal(fs)
	}
}

func BenchmarkFilterStats_Unmarshal(b *testing.B) {
	data := []byte(`{"capacity":100000,"itemCount":5000,"fillRatio":0.05,"falsePositiveRate":0.01,"storageType":"memory"}`)
	for b.Loop() {
		var fs stats.FilterStats
		_ = json.Unmarshal(data, &fs)
	}
}

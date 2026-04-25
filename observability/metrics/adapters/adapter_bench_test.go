// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package adapters

import "testing"

func BenchmarkMultiAdapter_RecordCounter(b *testing.B) {
	a1 := &testAdapter{name: "a1"}
	a2 := &testAdapter{name: "a2"}
	m := NewMultiAdapter(a1, a2)
	labels := map[string]string{"method": "GET"}
	b.ResetTimer()
	for b.Loop() {
		m.RecordCounter("requests", labels, 1)
	}
}

func BenchmarkMultiAdapter_RecordHistogram(b *testing.B) {
	a1 := &testAdapter{name: "a1"}
	m := NewMultiAdapter(a1)
	labels := map[string]string{"method": "GET"}
	b.ResetTimer()
	for b.Loop() {
		m.RecordHistogram("duration", labels, 0.1)
	}
}

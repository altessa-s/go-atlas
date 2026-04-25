// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import "testing"

func BenchmarkSortedKeys(b *testing.B) {
	labels := Labels{"method": "GET", "path": "/api", "status": "200", "host": "localhost"}
	b.ResetTimer()
	for b.Loop() {
		SortedKeys(labels)
	}
}

func BenchmarkSortedLabelValues(b *testing.B) {
	labels := Labels{"method": "GET", "path": "/api", "status": "200"}
	names := []string{"method", "path", "status"}
	b.ResetTimer()
	for b.Loop() {
		SortedLabelValues(labels, names)
	}
}

func BenchmarkLabelBuilder(b *testing.B) {
	for b.Loop() {
		NewLabelBuilder().
			Add("method", "GET").
			Add("path", "/api").
			Add("status", "200").
			Build()
	}
}

func BenchmarkCloneLabels(b *testing.B) {
	labels := Labels{"method": "GET", "path": "/api", "status": "200"}
	b.ResetTimer()
	for b.Loop() {
		CloneLabels(labels)
	}
}

func BenchmarkMergeLabels(b *testing.B) {
	base := Labels{"a": "1", "b": "2"}
	other := Labels{"c": "3", "d": "4"}
	b.ResetTimer()
	for b.Loop() {
		MergeLabels(base, other)
	}
}

func BenchmarkLabelsPool(b *testing.B) {
	for b.Loop() {
		l := GetLabels()
		(*l)["key"] = "val"
		PutLabels(l)
	}
}

func BenchmarkValidateLabels(b *testing.B) {
	labels := Labels{"method": "GET", "path": "/api", "status": "200"}
	names := []string{"method", "path", "status"}
	b.ResetTimer()
	for b.Loop() {
		_ = ValidateLabels(labels, names)
	}
}

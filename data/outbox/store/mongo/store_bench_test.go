// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

import "testing"

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

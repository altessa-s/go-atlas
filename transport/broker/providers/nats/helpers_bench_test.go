// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import "testing"

func BenchmarkIsValidSubject_Valid(b *testing.B) {
	for b.Loop() {
		IsValidSubject("my-topic-123")
	}
}

func BenchmarkIsValidSubject_Invalid(b *testing.B) {
	for b.Loop() {
		IsValidSubject("topic.with.dots")
	}
}

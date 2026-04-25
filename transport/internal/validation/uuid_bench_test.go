// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validation

import "testing"

func BenchmarkIsValidUUIDv4_Valid(b *testing.B) {
	id := "550e8400-e29b-41d4-a716-446655440000"
	for b.Loop() {
		IsValidUUIDv4(id)
	}
}

func BenchmarkIsValidUUIDv4_Invalid(b *testing.B) {
	id := "not-a-valid-uuid"
	for b.Loop() {
		IsValidUUIDv4(id)
	}
}

func BenchmarkIsValidUUIDv4_WrongLength(b *testing.B) {
	id := "short"
	for b.Loop() {
		IsValidUUIDv4(id)
	}
}

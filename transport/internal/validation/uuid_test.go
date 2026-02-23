// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validation

import "testing"

func TestIsValidUUIDv4(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid_lowercase", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid_uppercase", "550E8400-E29B-41D4-A716-446655440000", true},
		{"valid_variant_8", "550e8400-e29b-41d4-8716-446655440000", true},
		{"valid_variant_9", "550e8400-e29b-41d4-9716-446655440000", true},
		{"valid_variant_a", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid_variant_b", "550e8400-e29b-41d4-b716-446655440000", true},
		{"valid_variant_A", "550e8400-e29b-41d4-A716-446655440000", true},
		{"valid_variant_B", "550e8400-e29b-41d4-B716-446655440000", true},
		{"empty", "", false},
		{"too_short", "550e8400-e29b-41d4-a716", false},
		{"too_long", "550e8400-e29b-41d4-a716-4466554400001", false},
		{"wrong_version_1", "550e8400-e29b-11d4-a716-446655440000", false},
		{"wrong_version_5", "550e8400-e29b-51d4-a716-446655440000", false},
		{"wrong_variant_c", "550e8400-e29b-41d4-c716-446655440000", false},
		{"wrong_variant_0", "550e8400-e29b-41d4-0716-446655440000", false},
		{"no_hyphens", "550e8400e29b41d4a716446655440000xxxx", false},
		{"wrong_hyphen_pos", "550e840-0e29b-41d4-a716-446655440000", false},
		{"non_hex_char", "550e8400-e29b-41d4-a716-44665544000g", false},
		{"spaces", "550e8400 e29b 41d4 a716 446655440000", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidUUIDv4(tt.input); got != tt.want {
				t.Fatalf("IsValidUUIDv4(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestUUIDLength(t *testing.T) {
	if UUIDLength != 36 {
		t.Fatalf("UUIDLength = %d, want 36", UUIDLength)
	}
}

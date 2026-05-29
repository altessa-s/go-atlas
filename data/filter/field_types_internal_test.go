// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestKindAccepts is the exhaustive whitebox test that the higher-
// level evaluator tests delegate type-acceptance semantics to. PR-45
// review flagged that kindAccepts had no direct unit coverage — Bytes
// in particular is unreachable from the CEL grammar this parser
// supports, so it could only be verified via this table.
//
// Every (kind, sample value) combination is listed: matches return
// true, mismatches return false. A regression that flips a single
// case (e.g. FieldKindInt accidentally accepting float64) shows up
// here as a single failed row.
func TestKindAccepts(t *testing.T) {
	t.Parallel()

	now := time.Now()

	cases := []struct {
		name  string
		kind  FieldKind
		value any
		want  bool
	}{
		// FieldKindUnspecified accepts everything — the short-circuit
		// in callers normally prevents this from being reached, but
		// the fall-through keeps the safety guarantee if a future
		// caller bypasses the short-circuit.
		{"unspecified accepts string", FieldKindUnspecified, "anything", true},
		{"unspecified accepts nil", FieldKindUnspecified, nil, true},

		// FieldKindInt — int64 and uint64 ONLY; CEL parses whole
		// numbers as int64, so price >= 100 lands here.
		{"int accepts int64", FieldKindInt, int64(42), true},
		{"int accepts uint64", FieldKindInt, uint64(42), true},
		{"int rejects float64", FieldKindInt, float64(42), false},
		{"int rejects string", FieldKindInt, "42", false},
		{"int rejects bool", FieldKindInt, true, false},

		// FieldKindFloat — float64 plus int64/uint64. CEL parses
		// "100" as int64; treating it as a valid float lets
		// `price >= 100` work against a double field.
		{"float accepts float64", FieldKindFloat, float64(1.5), true},
		{"float accepts int64", FieldKindFloat, int64(1), true},
		{"float accepts uint64", FieldKindFloat, uint64(1), true},
		{"float rejects string", FieldKindFloat, "1.5", false},
		{"float rejects bool", FieldKindFloat, true, false},

		// FieldKindString
		{"string accepts string", FieldKindString, "alice", true},
		{"string rejects int64", FieldKindString, int64(1), false},
		{"string rejects bool", FieldKindString, true, false},

		// FieldKindBool
		{"bool accepts true", FieldKindBool, true, true},
		{"bool accepts false", FieldKindBool, false, true},
		{"bool rejects int64", FieldKindBool, int64(1), false},
		{"bool rejects string", FieldKindBool, "true", false},

		// FieldKindBytes — unreachable from CEL grammar today, so
		// this table is the only place these assertions live.
		{"bytes accepts []byte", FieldKindBytes, []byte("payload"), true},
		{"bytes rejects string", FieldKindBytes, "payload", false},
		{"bytes rejects int64", FieldKindBytes, int64(1), false},

		// FieldKindTimestamp
		{"timestamp accepts time.Time", FieldKindTimestamp, now, true},
		{"timestamp rejects string", FieldKindTimestamp, "2024-01-01", false},
		{"timestamp rejects int64", FieldKindTimestamp, int64(0), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, kindAccepts(tc.kind, tc.value))
		})
	}
}

// TestValueKindName confirms the human-readable label helper that
// powers the FieldKindMismatch error message. Operators see DSL terms
// (int / string / timestamp), not Go typenames (int64 / time.Time).
// A regression here makes error messages mention internals the
// operator doesn't recognize.
func TestValueKindName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"int64 → int", int64(1), "int"},
		{"uint64 → int", uint64(1), "int"},
		{"float64 → float", float64(1), "float"},
		{"string → string", "x", "string"},
		{"bool → bool", true, "bool"},
		{"[]byte → bytes", []byte{1}, "bytes"},
		{"time.Time → timestamp", time.Now(), "timestamp"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, valueKindName(tc.value))
		})
	}
}

// TestValueKindName_FallbackOnUnknownType pins the fallback path:
// values whose Go type doesn't map to any FieldKind get formatted
// with %T so the error stays informative even for surprises.
func TestValueKindName_FallbackOnUnknownType(t *testing.T) {
	t.Parallel()

	type customStruct struct{}
	got := valueKindName(customStruct{})
	require.Contains(t, got, "customStruct",
		"unknown Go types must surface via %%T so operators can still diagnose the problem")
}

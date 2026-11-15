// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package decision_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/decision"
)

func TestRedactSensitiveAttributes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{"authorization header", "Authorization", "Bearer abc", decision.RedactedAttributePlaceholder},
		{"token substring", "x-refresh-token", "t0ken", decision.RedactedAttributePlaceholder},
		{"password", "password", "hunter2", decision.RedactedAttributePlaceholder},
		{"non-sensitive kept", "tenant", "acme", "acme"},
		{"trace kept", "trace_id", "abc123", "abc123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, decision.RedactSensitiveAttributes(tc.key, tc.value))
		})
	}
}

// TestRecordSanitizesAttributes pins that with a sanitizer configured,
// sensitive attribute values are scrubbed before reaching the sink, non-sensitive
// values pass through, and the caller's map is not mutated.
func TestRecordSanitizesAttributes(t *testing.T) {
	t.Parallel()

	sink := &captureSink{}
	rec := decision.NewRecorder(sink, decision.WithAttributeSanitizer(decision.RedactSensitiveAttributes))

	attrs := map[string]string{"authorization": "Bearer secret", "tenant": "acme"}
	require.NoError(t, rec.Record(t.Context(), decision.Decision{Action: "act", Attributes: attrs}))

	require.Len(t, sink.got, 1)
	require.Equal(t, decision.RedactedAttributePlaceholder, sink.got[0].Attributes["authorization"])
	require.Equal(t, "acme", sink.got[0].Attributes["tenant"])

	// Caller's original map is untouched.
	require.Equal(t, "Bearer secret", attrs["authorization"])
}

// TestRecordRedactsSensitiveAttributesByDefault verifies the default: a recorder
// built without options scrubs secret-bearing attributes and leaves the caller's
// map untouched.
func TestRecordRedactsSensitiveAttributesByDefault(t *testing.T) {
	t.Parallel()

	sink := &captureSink{}
	rec := decision.NewRecorder(sink)

	attrs := map[string]string{"authorization": "Bearer secret", "tenant": "acme"}
	require.NoError(t, rec.Record(t.Context(), decision.Decision{Action: "act", Attributes: attrs}))

	require.Len(t, sink.got, 1)
	require.Equal(t, decision.RedactedAttributePlaceholder, sink.got[0].Attributes["authorization"])
	require.Equal(t, "acme", sink.got[0].Attributes["tenant"])
	require.Equal(t, "Bearer secret", attrs["authorization"])
}

// TestRecordNilSanitizerPassesAttributes verifies the opt-out: a nil sanitizer
// records attributes verbatim.
func TestRecordNilSanitizerPassesAttributes(t *testing.T) {
	t.Parallel()

	sink := &captureSink{}
	rec := decision.NewRecorder(sink, decision.WithAttributeSanitizer(nil))

	attrs := map[string]string{"authorization": "Bearer secret"}
	require.NoError(t, rec.Record(t.Context(), decision.Decision{Action: "act", Attributes: attrs}))

	require.Len(t, sink.got, 1)
	require.Equal(t, "Bearer secret", sink.got[0].Attributes["authorization"])
}

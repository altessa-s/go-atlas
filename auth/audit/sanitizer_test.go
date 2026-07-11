// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/audit"
)

func TestRedactSensitiveAttributes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{"authorization header", "Authorization", "Bearer abc", audit.RedactedAttributePlaceholder},
		{"token substring", "x-refresh-token", "t0ken", audit.RedactedAttributePlaceholder},
		{"password", "password", "hunter2", audit.RedactedAttributePlaceholder},
		{"non-sensitive kept", "tenant", "acme", "acme"},
		{"trace kept", "trace_id", "abc123", "abc123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, audit.RedactSensitiveAttributes(tc.key, tc.value))
		})
	}
}

// TestRecordSanitizesAttributes pins the opt-in fix: with a sanitizer configured,
// sensitive attribute values are scrubbed before reaching the sink, non-sensitive
// values pass through, and the caller's map is not mutated.
func TestRecordSanitizesAttributes(t *testing.T) {
	t.Parallel()

	sink := &captureSink{}
	rec := audit.NewRecorder(sink, audit.WithAttributeSanitizer(audit.RedactSensitiveAttributes))

	attrs := map[string]string{"authorization": "Bearer secret", "tenant": "acme"}
	require.NoError(t, rec.Record(t.Context(), audit.Decision{Action: "act", Attributes: attrs}))

	require.Len(t, sink.got, 1)
	require.Equal(t, audit.RedactedAttributePlaceholder, sink.got[0].Attributes["authorization"])
	require.Equal(t, "acme", sink.got[0].Attributes["tenant"])

	// Caller's original map is untouched.
	require.Equal(t, "Bearer secret", attrs["authorization"])
}

// TestRecordNoSanitizerPassesAttributes verifies the default: without a sanitizer,
// attributes reach the sink verbatim (backward-compatible).
func TestRecordNoSanitizerPassesAttributes(t *testing.T) {
	t.Parallel()

	sink := &captureSink{}
	rec := audit.NewRecorder(sink)

	attrs := map[string]string{"authorization": "Bearer secret"}
	require.NoError(t, rec.Record(t.Context(), audit.Decision{Action: "act", Attributes: attrs}))

	require.Len(t, sink.got, 1)
	require.Equal(t, "Bearer secret", sink.got[0].Attributes["authorization"])
}

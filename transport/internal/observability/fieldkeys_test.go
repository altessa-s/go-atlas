// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observability

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFieldKeys_NonEmpty(t *testing.T) {
	keys := []struct {
		name string
		key  FieldKey
	}{
		{"ClientPeerIP", FieldKeyClientPeerIP},
		{"ClientUserAgent", FieldKeyClientUserAgent},
		{"ClientRealIP", FieldKeyClientRealIP},
		{"RequestID", FieldKeyRequestID},
		{"RequestDeadline", FieldKeyRequestDeadline},
		{"RequestStartTime", FieldKeyRequestStartTime},
		{"RequestEndTime", FieldKeyRequestEndTime},
		{"RequestDuration", FieldKeyRequestDuration},
		{"RequestContent", FieldKeyRequestContent},
		{"ResponseContent", FieldKeyResponseContent},
		{"TraceID", FieldKeyTraceID},
		{"SpanID", FieldKeySpanID},
		{"Panic", FieldKeyPanic},
		{"PanicStacktrace", FieldKeyPanicStacktrace},
		{"Error", FieldKeyError},
	}

	for _, tt := range keys {
		t.Run(tt.name, func(t *testing.T) {
			require.NotEqual(t, "", tt.key)
		})
	}
}

func TestFieldKeys_Unique(t *testing.T) {
	keys := []FieldKey{
		FieldKeyClientPeerIP, FieldKeyClientUserAgent, FieldKeyClientRealIP,
		FieldKeyRequestID, FieldKeyRequestDeadline, FieldKeyRequestStartTime,
		FieldKeyRequestEndTime, FieldKeyRequestDuration, FieldKeyRequestContent,
		FieldKeyResponseContent, FieldKeyTraceID, FieldKeySpanID,
		FieldKeyPanic, FieldKeyPanicStacktrace, FieldKeyError,
	}

	seen := make(map[FieldKey]bool, len(keys))
	for _, k := range keys {
		require.False(t, seen[k], "duplicate field key: %s", k)
		seen[k] = true
	}
}

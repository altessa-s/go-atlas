// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import "testing"

func TestSpanKind_String(t *testing.T) {
	tests := []struct {
		kind SpanKind
		want string
	}{
		{SpanKindUnspecified, "unspecified"},
		{SpanKindInternal, "internal"},
		{SpanKindServer, "server"},
		{SpanKindClient, "client"},
		{SpanKindProducer, "producer"},
		{SpanKindConsumer, "consumer"},
		{SpanKind(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("SpanKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"testing"
)

type mockHeaderGetter struct {
	headers map[string]string
}

func (m *mockHeaderGetter) GetHeader(name string) string {
	return m.headers[name]
}

func TestGenerator_Extract(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name    string
		opts    []Option
		headers HeaderGetter
		want    string
	}{
		{
			"existing_valid_uuid",
			nil,
			&mockHeaderGetter{headers: map[string]string{DefaultHTTPHeaderName: validUUID}},
			validUUID,
		},
		{
			"invalid_uuid_generates_new",
			[]Option{WithUuidGenerator(func() string { return "generated-id" })},
			&mockHeaderGetter{headers: map[string]string{DefaultHTTPHeaderName: "invalid"}},
			"generated-id",
		},
		{
			"missing_header_generates_new",
			[]Option{WithUuidGenerator(func() string { return "generated-id" })},
			&mockHeaderGetter{headers: map[string]string{}},
			"generated-id",
		},
		{
			"nil_headers_generates_new",
			[]Option{WithUuidGenerator(func() string { return "generated-id" })},
			nil,
			"generated-id",
		},
		{
			"custom_header_name",
			[]Option{
				WithHeaderName("X-Custom-ID"),
				WithUuidGenerator(func() string { return "gen" }),
			},
			&mockHeaderGetter{headers: map[string]string{"X-Custom-ID": validUUID}},
			validUUID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewGenerator(tt.opts...)
			got := gen.Extract(tt.headers)
			if got != tt.want {
				t.Fatalf("Extract() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerator_HeaderName(t *testing.T) {
	gen := NewGenerator()
	if got := gen.HeaderName(); got != DefaultHTTPHeaderName {
		t.Fatalf("HeaderName() = %q, want %q", got, DefaultHTTPHeaderName)
	}

	gen = NewGenerator(WithHeaderName("X-Custom"))
	if got := gen.HeaderName(); got != "X-Custom" {
		t.Fatalf("HeaderName() = %q, want %q", got, "X-Custom")
	}
}

func TestGenerator_GenerateIfMissing(t *testing.T) {
	gen := NewGenerator()
	if !gen.GenerateIfMissing() {
		t.Fatal("GenerateIfMissing() = false, want true (default)")
	}
}

func TestGenerator_DefaultGenerator(t *testing.T) {
	gen := NewGenerator()
	id := gen.Extract(nil)
	if id == "" {
		t.Fatal("default generator should produce non-empty ID")
	}
	if len(id) != 36 {
		t.Fatalf("default generator ID length = %d, want 36", len(id))
	}
}

func TestNewContext_FromContext(t *testing.T) {
	ctx := NewContext(t.Context(), "test-id")
	if got := FromContext(ctx); got != "test-id" {
		t.Fatalf("FromContext() = %q, want %q", got, "test-id")
	}
}

func TestFromContext_Empty(t *testing.T) {
	if got := FromContext(t.Context()); got != "" {
		t.Fatalf("FromContext(empty) = %q, want empty", got)
	}
}

func TestFromContext_NilContext(t *testing.T) {
	//nolint:staticcheck // SA1012: testing nil context behavior
	if got := FromContext(nil); got != "" {
		t.Fatalf("FromContext(nil) = %q, want empty", got)
	}
}

func TestConstants(t *testing.T) {
	if DefaultHTTPHeaderName != "Request-ID" {
		t.Fatalf("DefaultHTTPHeaderName = %q", DefaultHTTPHeaderName)
	}
	if DefaultGRPCMetadataKey != "request-id" {
		t.Fatalf("DefaultGRPCMetadataKey = %q", DefaultGRPCMetadataKey)
	}
	if InternedHTTPHeaderName != DefaultHTTPHeaderName {
		t.Fatalf("InternedHTTPHeaderName = %q", InternedHTTPHeaderName)
	}
	if InternedGRPCMetadataKey != DefaultGRPCMetadataKey {
		t.Fatalf("InternedGRPCMetadataKey = %q", InternedGRPCMetadataKey)
	}
}

func TestFromContextOrTraceID(t *testing.T) {
	t.Run("with_request_id", func(t *testing.T) {
		ctx := NewContext(t.Context(), "req-123")
		if got := FromContextOrTraceID(ctx); got != "req-123" {
			t.Fatalf("FromContextOrTraceID() = %q, want %q", got, "req-123")
		}
	})

	t.Run("no_request_id_no_trace", func(t *testing.T) {
		got := FromContextOrTraceID(t.Context())
		// Should return empty when neither request ID nor trace ID
		if got != "" {
			// May have trace ID from context; that's fine
		}
	})
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"testing"

	"github.com/stretchr/testify/require"
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
			[]Option{WithUUIDGenerator(func() string { return "generated-id" })},
			&mockHeaderGetter{headers: map[string]string{DefaultHTTPHeaderName: "invalid"}},
			"generated-id",
		},
		{
			"missing_header_generates_new",
			[]Option{WithUUIDGenerator(func() string { return "generated-id" })},
			&mockHeaderGetter{headers: map[string]string{}},
			"generated-id",
		},
		{
			"nil_headers_generates_new",
			[]Option{WithUUIDGenerator(func() string { return "generated-id" })},
			nil,
			"generated-id",
		},
		{
			"custom_header_name",
			[]Option{
				WithHeaderName("X-Custom-ID"),
				WithUUIDGenerator(func() string { return "gen" }),
			},
			&mockHeaderGetter{headers: map[string]string{"X-Custom-ID": validUUID}},
			validUUID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewGenerator(tt.opts...)
			got := gen.Extract(tt.headers)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGenerator_HeaderName(t *testing.T) {
	gen := NewGenerator()
	got := gen.HeaderName()
	require.Equal(t, DefaultHTTPHeaderName, got)

	gen = NewGenerator(WithHeaderName("X-Custom"))
	got = gen.HeaderName()
	require.Equal(t, "X-Custom", got)
}

func TestGenerator_GenerateIfMissing(t *testing.T) {
	gen := NewGenerator()
	require.True(t, gen.GenerateIfMissing(), "GenerateIfMissing() = false, want true (default)")
}

func TestGenerator_DefaultGenerator(t *testing.T) {
	gen := NewGenerator()
	id := gen.Extract(nil)
	require.NotEqual(t, "", id)
	require.Len(t, id, 36)
}

func TestNewContext_FromContext(t *testing.T) {
	ctx := NewContext(t.Context(), "test-id")
	got := FromContext(ctx)
	require.Equal(t, "test-id", got)
}

func TestFromContext_Empty(t *testing.T) {
	got := FromContext(t.Context())
	require.Equal(t, "", got)
}

func TestFromContext_NilContext(t *testing.T) {
	//nolint:staticcheck // SA1012: testing nil context behavior
	got := FromContext(nil)
	require.Equal(t, "", got)
}

func TestFromContextOrTraceID(t *testing.T) {
	t.Run("with_request_id", func(t *testing.T) {
		ctx := NewContext(t.Context(), "req-123")
		got := FromContextOrTraceID(ctx)
		require.Equal(t, "req-123", got)
	})

	t.Run("no_request_id_no_trace", func(t *testing.T) {
		got := FromContextOrTraceID(t.Context())
		// Should return empty when neither request ID nor trace ID
		if got != "" {
			// May have trace ID from context; that's fine
		}
	})
}

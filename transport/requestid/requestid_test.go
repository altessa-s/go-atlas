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
	require.Equal(t, "X-Request-ID", got, "the de-facto standard header, as the HTTP and gRPC configs default to")

	gen = NewGenerator(WithHeaderName("X-Custom"))
	got = gen.HeaderName()
	require.Equal(t, "X-Custom", got)
}

func TestGenerator_GenerateIfMissing(t *testing.T) {
	gen := NewGenerator()
	require.True(t, gen.GenerateIfMissing(), "GenerateIfMissing() = false, want true (default)")

	disabled := NewGenerator(WithGenerateIfMissing(false))
	require.False(t, disabled.GenerateIfMissing())
	require.Empty(t, disabled.Extract(nil), "a disabled generator must not mint an ID")
	require.Empty(t, disabled.Extract(&mockHeaderGetter{headers: map[string]string{DefaultHTTPHeaderName: "not-a-uuid"}}),
		"a disabled generator must drop an invalid ID without replacing it")

	const valid = "550e8400-e29b-41d4-a716-446655440000"
	require.Equal(t, valid, disabled.Extract(&mockHeaderGetter{headers: map[string]string{DefaultHTTPHeaderName: valid}}),
		"a disabled generator still accepts a valid incoming ID")
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

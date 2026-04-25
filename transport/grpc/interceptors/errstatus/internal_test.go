// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErrorTypeString(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "<nil>"},
		{"plain", errors.New("x"), "*errors.errorString"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errorTypeString(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestErrorKey(t *testing.T) {
	got := errorKey(nil)
	require.Equal(t, "", got)
	got = errorKey(errors.New("test"))
	require.NotEqual(t, "", got)
}

func TestSentinelErrorKey(t *testing.T) {
	got := sentinelErrorKey(nil)
	require.Equal(t, "", got)
	got = sentinelErrorKey(errors.New("sentinel"))
	require.NotEqual(t, "", got)
}

func TestFindSentinelError(t *testing.T) {
	s1 := errors.New("s1")
	s2 := errors.New("s2")

	tests := []struct {
		name      string
		err       error
		sentinels []error
		want      error
	}{
		{"nil_error", nil, []error{s1, s2}, nil},
		{"match_first", s1, []error{s1, s2}, s1},
		{"match_second", s2, []error{s1, s2}, s2},
		{"no_match", errors.New("other"), []error{s1, s2}, nil},
		{"nil_sentinel_entry", s1, []error{nil, s1}, s1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findSentinelError(tt.err, tt.sentinels)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBuildStatusConverterIndex(t *testing.T) {
	t.Run("nil_input", func(t *testing.T) {
		idx := buildStatusConverterIndex(nil)
		require.Nil(t, idx)
	})

	t.Run("simple_code_matcher_goes_to_fast_path", func(t *testing.T) {
		idx := buildStatusConverterIndex([]StatusConverter{{
			Matcher: func(ctx context.Context, st *status.Status) bool { return st.Code() == codes.NotFound },
			Convert: func(ctx context.Context, st *status.Status) error { return errors.New("not found") },
		}})
		require.NotNil(t, idx, "index should not be nil")
		require.Len(t, idx.fastPath[codes.NotFound], 1)
		require.Len(t, idx.slowPath, 0)
	})

	t.Run("complex_matcher_goes_to_slow_path", func(t *testing.T) {
		idx := buildStatusConverterIndex([]StatusConverter{{
			Matcher: func(ctx context.Context, st *status.Status) bool { return true },
			Convert: func(ctx context.Context, st *status.Status) error { return errors.New("always") },
		}})
		require.NotNil(t, idx, "index should not be nil")
		require.Len(t, idx.slowPath, 1)
	})
}

func TestTryExtractCodeFromMatcher(t *testing.T) {
	tests := []struct {
		name    string
		matcher func(context.Context, *status.Status) bool
		want    codes.Code
	}{
		{
			"single_code_match",
			func(ctx context.Context, st *status.Status) bool { return st.Code() == codes.PermissionDenied },
			codes.PermissionDenied,
		},
		{
			"multiple_codes_returns_unknown",
			func(ctx context.Context, st *status.Status) bool {
				return st.Code() == codes.NotFound || st.Code() == codes.Internal
			},
			codes.Unknown,
		},
		{
			"no_match_returns_unknown",
			func(ctx context.Context, st *status.Status) bool { return false },
			codes.Unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tryExtractCodeFromMatcher(tt.matcher)
			require.Equal(t, tt.want, got)
		})
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"testing"

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
			if got := errorTypeString(tt.err); got != tt.want {
				t.Fatalf("errorTypeString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorKey(t *testing.T) {
	if got := errorKey(nil); got != "" {
		t.Fatalf("errorKey(nil) = %q, want empty", got)
	}
	if got := errorKey(errors.New("test")); got == "" {
		t.Fatal("errorKey should not be empty for non-nil error")
	}
}

func TestSentinelErrorKey(t *testing.T) {
	if got := sentinelErrorKey(nil); got != "" {
		t.Fatalf("sentinelErrorKey(nil) = %q, want empty", got)
	}
	if got := sentinelErrorKey(errors.New("sentinel")); got == "" {
		t.Fatal("sentinelErrorKey should not be empty for non-nil error")
	}
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
			if got := findSentinelError(tt.err, tt.sentinels); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildStatusConverterIndex(t *testing.T) {
	t.Run("nil_input", func(t *testing.T) {
		if idx := buildStatusConverterIndex(nil); idx != nil {
			t.Fatal("nil input should return nil index")
		}
	})

	t.Run("simple_code_matcher_goes_to_fast_path", func(t *testing.T) {
		idx := buildStatusConverterIndex([]StatusConverter{{
			Matcher: func(ctx context.Context, st *status.Status) bool { return st.Code() == codes.NotFound },
			Convert: func(ctx context.Context, st *status.Status) error { return errors.New("not found") },
		}})
		if idx == nil {
			t.Fatal("index should not be nil")
		}
		if len(idx.fastPath[codes.NotFound]) != 1 {
			t.Fatalf("fastPath[NotFound] len = %d, want 1", len(idx.fastPath[codes.NotFound]))
		}
		if len(idx.slowPath) != 0 {
			t.Fatalf("slowPath len = %d, want 0", len(idx.slowPath))
		}
	})

	t.Run("complex_matcher_goes_to_slow_path", func(t *testing.T) {
		idx := buildStatusConverterIndex([]StatusConverter{{
			Matcher: func(ctx context.Context, st *status.Status) bool { return true },
			Convert: func(ctx context.Context, st *status.Status) error { return errors.New("always") },
		}})
		if idx == nil {
			t.Fatal("index should not be nil")
		}
		if len(idx.slowPath) != 1 {
			t.Fatalf("slowPath len = %d, want 1", len(idx.slowPath))
		}
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
			if got := tryExtractCodeFromMatcher(tt.matcher); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

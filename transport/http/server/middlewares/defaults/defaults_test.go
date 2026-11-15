// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIgnorePatterns(t *testing.T) {
	require.NotEmpty(t, IgnorePatterns)
	for _, p := range IgnorePatterns {
		require.NotNil(t, p)
	}
}

func TestIgnorePatterns_MatchHealthAndMetrics(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/healthz", true},
		{"/metrics", true},
		{"/internal/readyz", true},
		{"/internal/metrics/", true},
		{"/debug/pprof/heap", true},
		{"/api/users", false},
		{"/users/metrics", false},
		{"/x/pprof/y", false},
		{"/admin/healthz", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			require.Equal(t, tt.want, matchAnyOf(IgnorePatterns, tt.path))
		})
	}
}

func TestIgnorePatternsWithPrefix(t *testing.T) {
	t.Parallel()

	patterns := IgnorePatternsWithPrefix("/ops")
	require.True(t, matchAnyOf(patterns, "/ops/healthz"))
	require.True(t, matchAnyOf(patterns, "/metrics"))
	require.False(t, matchAnyOf(patterns, "/internal/healthz"))
	require.False(t, matchAnyOf(patterns, "/users/ops/healthz"))
}

func matchAnyOf(patterns []*regexp.Regexp, path string) bool {
	for _, p := range patterns {
		if p.MatchString(path) {
			return true
		}
	}
	return false
}

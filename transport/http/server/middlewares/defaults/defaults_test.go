// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import (
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
		{"/api/users", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			matched := false
			for _, p := range IgnorePatterns {
				if p.MatchString(tt.path) {
					matched = true
					break
				}
			}
			require.Equal(t, tt.want, matched)
		})
	}
}

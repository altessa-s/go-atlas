// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSemVer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		in         string
		wantMajor  string
		wantMinor  string
		wantPatch  string
		wantPre    []string
		shouldFail bool
	}{
		{
			name:      "v prefix and prerelease and build",
			in:        "v1.2.3-alpha.1+build.5",
			wantMajor: "1",
			wantMinor: "2",
			wantPatch: "3",
			wantPre:   []string{"alpha", "1"},
		},
		{
			name:      "major only defaults",
			in:        "1",
			wantMajor: "1",
			wantMinor: "0",
			wantPatch: "0",
		},
		{
			name:       "invalid too many parts",
			in:         "1.2.3.4",
			shouldFail: true,
		},
		{
			name:       "empty",
			in:         "",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSemVer(tt.in)
			if tt.shouldFail {
				require.Error(t, err, "expected error for input %q", tt.in)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantMajor, got.Major)
			require.Equal(t, tt.wantMinor, got.Minor)
			require.Equal(t, tt.wantPatch, got.Patch)
			require.Len(t, got.Prerelease, len(tt.wantPre))
			for i := range tt.wantPre {
				require.Equal(t, tt.wantPre[i], got.Prerelease[i], "prerelease[%d]", i)
			}
		})
	}
}

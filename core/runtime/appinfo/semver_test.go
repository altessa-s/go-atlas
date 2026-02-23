// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo

import "testing"

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
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSemVer err=%v", err)
			}
			if got.Major != tt.wantMajor || got.Minor != tt.wantMinor || got.Patch != tt.wantPatch {
				t.Fatalf("got=%s.%s.%s, want=%s.%s.%s", got.Major, got.Minor, got.Patch, tt.wantMajor, tt.wantMinor, tt.wantPatch)
			}
			if len(got.Prerelease) != len(tt.wantPre) {
				t.Fatalf("prerelease len=%d, want=%d (%v)", len(got.Prerelease), len(tt.wantPre), got.Prerelease)
			}
			for i := range tt.wantPre {
				if got.Prerelease[i] != tt.wantPre[i] {
					t.Fatalf("prerelease[%d]=%q, want %q", i, got.Prerelease[i], tt.wantPre[i])
				}
			}
		})
	}
}

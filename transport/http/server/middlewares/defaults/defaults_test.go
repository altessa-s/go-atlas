// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import "testing"

func TestIgnorePatterns(t *testing.T) {
	if len(IgnorePatterns) == 0 {
		t.Fatal("IgnorePatterns should not be empty")
	}
	for i, p := range IgnorePatterns {
		if p == nil {
			t.Fatalf("IgnorePatterns[%d] is nil", i)
		}
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
			if matched != tt.want {
				t.Fatalf("path %q: matched=%v, want=%v", tt.path, matched, tt.want)
			}
		})
	}
}

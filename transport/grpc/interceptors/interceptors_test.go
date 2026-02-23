// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import "testing"

func TestMatchFunc(t *testing.T) {
	tests := []struct {
		name string
		fn   MatchFunc
		want bool
	}{
		{"true", func() bool { return true }, true},
		{"false", func() bool { return false }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn.Match(); got != tt.want {
				t.Fatalf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIgnorePatterns(t *testing.T) {
	require.Len(t, IgnorePatterns, 2)
}

func TestIgnorePatterns_MatchesReflection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"reflection", "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo", true},
		{"health", "/grpc.health.v1.Health/Check", true},
		{"normal", "/mypackage.MyService/MyMethod", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := false
			for _, p := range IgnorePatterns {
				if p.MatchString(tt.input) {
					matched = true
					break
				}
			}
			require.Equal(t, tt.want, matched)
		})
	}
}

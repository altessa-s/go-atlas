// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fallback_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/policy/fallback"
)

func TestParseBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want fallback.Behavior
	}{
		{name: "allow", in: "allow", want: fallback.Allow},
		{name: "deny", in: "deny", want: fallback.Deny},
		{name: "error", in: "error", want: fallback.Error},
		{name: "empty fails closed", in: "", want: fallback.Deny},
		{name: "unknown fails closed", in: "permit", want: fallback.Deny},
		{name: "case sensitive fails closed", in: "ALLOW", want: fallback.Deny},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := fallback.ParseBehavior(tc.in)
			require.Equal(t, tc.want, got)
			require.True(t, got.IsValid())
		})
	}
}

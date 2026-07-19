// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildKeyStorageABIPinned pins the exact storage-key format as an
// external ABI: keys built here persist in Redis/NATS/memory backends, so
// the literal byte layout must never change across refactors. The expected
// values are hard-coded string literals on purpose — do not derive them
// from the same helper that produces them.
func TestBuildKeyStorageABIPinned(t *testing.T) {
	t.Parallel()

	m := New(nil)

	tests := []struct {
		name   string
		method string
		path   string
		key    string
		want   string
	}{
		{
			name:   "canonical POST path",
			method: "POST",
			path:   "/api/v1/users",
			key:    "550e8400-e29b-41d4-a716-446655440000",
			want:   "idk:POST:/api/v1/users:550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:   "path without leading slash is normalized",
			method: "PUT",
			path:   "orders",
			key:    "550e8400-e29b-41d4-a716-446655440000",
			want:   "idk:PUT:/orders:550e8400-e29b-41d4-a716-446655440000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, m.buildKey(tc.method, tc.path, tc.key))
		})
	}
}

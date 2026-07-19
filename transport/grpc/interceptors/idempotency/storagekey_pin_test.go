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

	ri := &requestInterceptor{}

	tests := []struct {
		name   string
		method string
		key    string
		want   string
	}{
		{
			name:   "fully qualified method with leading slash",
			method: "/users.UserService/CreateUser",
			key:    "550e8400-e29b-41d4-a716-446655440000",
			want:   "idk:users.UserService:550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:   "fully qualified method without leading slash",
			method: "users.UserService/CreateUser",
			key:    "550e8400-e29b-41d4-a716-446655440000",
			want:   "idk:users.UserService:550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:   "bare service name without method part",
			method: "users.UserService",
			key:    "550e8400-e29b-41d4-a716-446655440000",
			want:   "idk:users.UserService:550e8400-e29b-41d4-a716-446655440000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, ri.buildKey(tc.method, tc.key))
		})
	}
}

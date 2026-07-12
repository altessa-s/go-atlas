// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestScrubURLCredentials pins the guarantee that credentials embedded in a NATS
// connect error string are removed before the error is returned/logged.
func TestScrubURLCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  string
		url  string
		want string
	}{
		{
			name: "single host userinfo stripped",
			msg:  "nats: connection failed for nats://user:pass@host:4222",
			url:  "nats://user:pass@host:4222",
			want: "nats: connection failed for nats://host:4222",
		},
		{
			name: "multi host list, per-component scrub",
			msg:  "no servers available: nats://a:secret@h1:4222, nats://b:top@h2:4222",
			url:  "nats://a:secret@h1:4222,nats://b:top@h2:4222",
			want: "no servers available: nats://h1:4222, nats://h2:4222",
		},
		{
			name: "no userinfo left untouched",
			msg:  "timeout connecting to nats://host:4222",
			url:  "nats://host:4222",
			want: "timeout connecting to nats://host:4222",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scrubURLCredentials(tc.msg, tc.url)
			require.Equal(t, tc.want, got)
			require.NotContains(t, got, "pass")
			require.NotContains(t, got, "secret")
			require.NotContains(t, got, "top")
		})
	}
}

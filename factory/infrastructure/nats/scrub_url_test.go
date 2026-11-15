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

// TestScrubURLCredentials_PasswordFragments pins that a password nats.go cannot
// parse does not leak through the parse error, which quotes a fragment of the
// URL and echoes part of the password as an "invalid port". The messages are
// the ones nats.Connect returns for these URLs.
func TestScrubURLCredentials_PasswordFragments(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		msg  string
		url  string
	}{
		{
			name: "comma in password",
			msg:  `parse "nats://u:s3cr": invalid port ":s3cr" after host`,
			url:  "nats://u:s3cr,etXYZ@127.0.0.1:4222",
		},
		{
			name: "comma in password, numeric first fragment",
			msg:  `dial tcp: lookup u: no such host (nats://u:4222)`,
			url:  "nats://u:4222,etXYZ@127.0.0.1:4222",
		},
		{
			name: "slash in password",
			msg:  `parse "nats://u:s3cr/etXYZ@127.0.0.1:4222": invalid port ":s3cr" after host`,
			url:  "nats://u:s3cr/etXYZ@127.0.0.1:4222",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scrubURLCredentials(tc.msg, tc.url)
			require.NotContains(t, got, "s3cr")
			require.NotContains(t, got, "4222)")
			require.NotContains(t, got, "etXYZ")
		})
	}
}

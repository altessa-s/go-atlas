// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactNATSURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain host:port unchanged",
			in:   "nats://localhost:4222",
			want: "nats://localhost:4222",
		},
		{
			name: "user:pass stripped",
			in:   "nats://user:s3cret@host:4222",
			want: "nats://host:4222",
		},
		{
			name: "user-only stripped",
			in:   "nats://user@host:4222",
			want: "nats://host:4222",
		},
		{
			name: "multi-url list — each component redacted independently",
			in:   "nats://u1:p1@host1:4222,nats://u2:p2@host2:4222",
			want: "nats://host1:4222,nats://host2:4222",
		},
		{
			name: "garbage input passes through",
			in:   "not-a-url",
			want: "not-a-url",
		},
		{
			name: "unparsable URL still loses userinfo",
			in:   "nats://user:s3cr3t@[::1",
			want: "nats://[::1",
		},
		{
			name: "unencoded slash and at-sign in password",
			in:   "nats://user:pa/s@s@host:4222",
			want: "nats://host:4222",
		},
		{
			name: "schemeless host with userinfo",
			in:   "user:s3cr3t@host:4222",
			want: "host:4222",
		},
		{
			name: "comma in password",
			in:   "nats://user:s3cr,etXYZ@host:4222",
			want: "nats://host:4222",
		},
		{
			name: "comma in password inside a list",
			in:   "nats://u1:p1@host1:4222,nats://u2:s3cr,et,XYZ@host2:4222,nats://host3:4222",
			want: "nats://host1:4222,nats://host2:4222,nats://host3:4222",
		},
		{
			name: "schemeless list without userinfo unchanged",
			in:   "nats://host1:4222,host2:4222",
			want: "nats://host1:4222,host2:4222",
		},
		{
			name: "mixed list with one unparsable component",
			in:   "nats://u1:p1@host1:4222, nats://u2:p2@[::1",
			want: "nats://host1:4222, nats://[::1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, redactNATSURL(tc.in))
		})
	}
}

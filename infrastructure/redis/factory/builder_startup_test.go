// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"

	redisconfig "github.com/altessa-s/go-atlas/config/redis"
)

// closedAddr returns a loopback address on which nothing listens.
func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func TestBuild_RedisDown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		configure      func(*ClientBuilder) *ClientBuilder
		wantErr        bool
		wantRegistered bool
	}{
		{name: "default pings and fails", configure: func(b *ClientBuilder) *ClientBuilder { return b }, wantErr: true},
		{name: "skip ping registers health", configure: (*ClientBuilder).SkipStartupPing, wantRegistered: true},
		{
			name:      "skip ping without health",
			configure: func(b *ClientBuilder) *ClientBuilder { return b.SkipStartupPing().WithoutHealth() },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			coord := health.New()
			t.Cleanup(coord.Close)
			b := New(&redisconfig.Config{Hosts: []string{closedAddr(t)}}).UseHealthCoordinator(coord)

			client, err := tc.configure(b).Build(t.Context())
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, client)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, client)
			t.Cleanup(func() { _ = client.Close() })

			// The client exists, but commands fail until Redis is reachable.
			require.Error(t, client.Ping(t.Context()).Err())
			require.Equal(t, tc.wantRegistered, slices.Contains(slices.Collect(coord.ListServices()), "redis"))
		})
	}
}

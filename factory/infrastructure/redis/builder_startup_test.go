// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net"
	"slices"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
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
			require.Error(t, client.Client().Ping(t.Context()).Err())
			require.Equal(t, tc.wantRegistered, slices.Contains(slices.Collect(coord.ListServices()), "redis"))
		})
	}
}

func TestBuildCloseReleasesOnlyOwnedChecker(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)
	cfg := redisconfig.Default()
	first, err := New(&cfg).SkipStartupPing().UseHealthCoordinator(struct{ health.ScopedReporter }{coord}).Build(t.Context())
	require.NoError(t, err)
	second, err := New(&cfg).SkipStartupPing().UseHealthCoordinator(struct{ health.ScopedReporter }{coord}).Build(t.Context())
	require.NoError(t, err)
	require.IsType(t, (*redis.Client)(nil), second.Client())
	require.Equal(t, []string{"redis"}, slices.Collect(coord.ListServices()))
	require.NoError(t, first.Close())
	require.NoError(t, first.Close())
	require.Equal(t, []string{"redis"}, slices.Collect(coord.ListServices()))
	require.NoError(t, second.Close())
	require.Empty(t, slices.Collect(coord.ListServices()))
	require.Equal(t, health.StatusServing, coord.CheckHealth(t.Context()))
}

func TestBuildShutdownClosesClient(t *testing.T) {
	t.Parallel()
	var hooks coreruntime.HookGroup
	cfg := redisconfig.Default()
	coord := health.New()
	t.Cleanup(coord.Close)
	owner, err := New(&cfg).SkipStartupPing().UseShutdownHooks(&hooks).UseHealthCoordinator(coord).Build(t.Context())
	require.NoError(t, err)
	require.NoError(t, hooks.Shutdown(t.Context()))
	require.Empty(t, slices.Collect(coord.ListServices()))
	require.ErrorIs(t, owner.Client().Ping(t.Context()).Err(), redis.ErrClosed)
	require.NoError(t, owner.Close())
	require.NoError(t, owner.Close())
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/client/dnsresolver"

	grpcclientconfig "github.com/altessa-s/go-atlas/config/grpcclient"
	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
)

func TestConnectionOptions(t *testing.T) {
	t.Parallel()

	builtin := grpcclientconfig.Default()
	builtin.Resolver.Builtin = true
	connectOnly := grpcclientconfig.Config{ConnectBackoff: grpcclientconfig.ConnectBackoff{MaxDelay: 10 * time.Second}}
	resolverOnly := grpcclientconfig.Config{Resolver: grpcclientconfig.Resolver{Backoff: grpcclientconfig.ResolverBackoff{Jitter: new(0.0)}}}
	full := grpcclientconfig.Default()

	tests := []struct {
		name string
		cfg  *grpcclientconfig.Config
		want int
	}{
		{"nil", nil, 0},
		{"zero keeps the client defaults", &grpcclientconfig.Config{}, 0},
		{"builtin resolver ignores the resolver settings", &builtin, 2},
		{"connection backoff only", &connectOnly, 1},
		{"explicit zero jitter", &resolverOnly, 1},
		{"defaults", &full, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := ConnectionOptions(tc.cfg)
			require.Len(t, opts, tc.want)
			if tc.cfg == nil {
				require.Nil(t, opts)
			}
		})
	}
}

func TestResolverOptionsMapSetFields(t *testing.T) {
	t.Parallel()

	require.Empty(t, resolverOptions(&grpcclientconfig.Resolver{}))
	all := grpcclientconfig.DefaultResolver()
	require.Len(t, resolverOptions(&all), 6)
	all.Backoff.Jitter = new(0.0)
	require.Len(t, resolverOptions(&all), 7, "an explicit zero jitter disables jitter")
	all.RefreshJitter = new(0.0)
	require.Len(t, resolverOptions(&all), 8)

	require.Empty(t, connectBackoffOptions(&grpcclientconfig.ConnectBackoff{}))
	backoff := grpcclientconfig.DefaultConnectBackoff()
	require.Len(t, connectBackoffOptions(&backoff), 3)
	backoff.Jitter = new(0.5)
	require.Len(t, connectBackoffOptions(&backoff), 4)
}

// The schema documents the defaults a zero field keeps; they must be the
// client's own.
func TestSchemaDefaultsMatchTheClient(t *testing.T) {
	t.Parallel()

	r := grpcclientconfig.DefaultResolver()
	require.Equal(t, dnsresolver.DefaultRefreshInterval, r.RefreshInterval)
	require.Equal(t, dnsresolver.DefaultMinInterval, r.MinInterval)
	require.Equal(t, dnsresolver.DefaultLookupTimeout, r.LookupTimeout)
	require.Equal(t, dnsresolver.DefaultBackoffBaseDelay, r.Backoff.BaseDelay)
	require.InDelta(t, dnsresolver.DefaultBackoffMultiplier, r.Backoff.Multiplier, 0)
	require.Equal(t, dnsresolver.DefaultBackoffMaxDelay, r.Backoff.MaxDelay)
	require.InDelta(t, 0.2, dnsresolver.DefaultBackoffJitter, 0, "the schema documents an unset jitter as 0.2")
	require.InDelta(t, 0.1, dnsresolver.DefaultRefreshJitter, 0, "the schema documents an unset refresh jitter as 0.1")
	require.InDelta(t, 0.5, dnsresolver.MaxRefreshJitter, 0, "the schema caps the refresh jitter at 0.5")

	b := grpcclientconfig.DefaultConnectBackoff()
	require.Equal(t, grpcclient.DefaultConnectBackoffBaseDelay, b.BaseDelay)
	require.InDelta(t, grpcclient.DefaultConnectBackoffMultiplier, b.Multiplier, 0)
	require.Equal(t, grpcclient.DefaultConnectBackoffMaxDelay, b.MaxDelay)
	require.InDelta(t, 0.2, grpcclient.DefaultConnectBackoffJitter, 0, "the schema documents an unset jitter as 0.2")
}

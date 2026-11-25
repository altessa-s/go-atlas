// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

// White-box: the connection parameters are private dial policy with no
// observable accessor, so the test reads what dialOptions hands to gRPC.
func TestConnectParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []Option
		want backoff.Config
	}{
		{"default", nil, backoff.DefaultConfig},
		{"no options", []Option{WithConnectBackoff()}, backoff.DefaultConfig},
		{"custom", []Option{WithConnectBackoff(
			WithConnectBackoffBaseDelay(100*time.Millisecond),
			WithConnectBackoffMultiplier(2),
			WithConnectBackoffJitter(0),
			WithConnectBackoffMaxDelay(5*time.Second),
		)}, backoff.Config{BaseDelay: 100 * time.Millisecond, Multiplier: 2, Jitter: 0, MaxDelay: 5 * time.Second}},
		{"invalid values keep defaults", []Option{WithConnectBackoff(
			WithConnectBackoffBaseDelay(-time.Second),
			WithConnectBackoffMultiplier(0),
			WithConnectBackoffJitter(-0.5),
			WithConnectBackoffMaxDelay(0),
		)}, backoff.DefaultConfig},
		{"later call replaces", []Option{
			WithConnectBackoff(WithConnectBackoffMaxDelay(time.Second)),
			WithConnectBackoff(WithConnectBackoffBaseDelay(2 * time.Second)),
		}, backoff.Config{BaseDelay: 2 * time.Second, Multiplier: 1.6, Jitter: 0.2, MaxDelay: 120 * time.Second}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &Client{options: *newOptions(tc.opts...)}
			require.Equal(t, grpc.ConnectParams{Backoff: tc.want, MinConnectTimeout: MinConnectTimeout}, c.connectParams())
		})
	}
	require.Equal(t, backoff.DefaultConfig, (*connectBackoffOptions)(nil).config())
}

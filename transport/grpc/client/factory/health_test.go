// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/grpc/client/factory"
)

func TestGRPCHealthClientOptions(t *testing.T) {
	t.Parallel()

	t.Run("nil returns nil", func(t *testing.T) {
		t.Parallel()

		var h *config.GRPCHealthClient
		opts := factory.HealthOptions(h)
		require.Nil(t, opts)
	})

	t.Run("empty config returns empty options", func(t *testing.T) {
		t.Parallel()

		h := &config.GRPCHealthClient{}
		opts := factory.HealthOptions(h)
		require.Empty(t, opts)
	})

	t.Run("with service name returns options", func(t *testing.T) {
		t.Parallel()

		h := &config.GRPCHealthClient{
			HealthClient: config.HealthClient{
				ServiceName: "test-grpc-service",
			},
		}
		opts := factory.HealthOptions(h)
		require.Len(t, opts, 1)
	})

	t.Run("ignores HTTP-specific fields", func(t *testing.T) {
		t.Parallel()

		h := &config.GRPCHealthClient{
			HealthClient: config.HealthClient{
				ServiceName: "test-grpc-service",
			},
			StateMapper: config.HealthClientStateMapperStrict, // This should not generate an option (yet)
			PerTarget:   true,                                 // This should not generate an option (yet)
		}
		opts := factory.HealthOptions(h)
		// Only service name should generate an option
		require.Len(t, opts, 1)
	})
}

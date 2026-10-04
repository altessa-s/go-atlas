// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/http/client/factory"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestHTTPHealthClientOptions(t *testing.T) {
	t.Parallel()

	t.Run("nil returns nil", func(t *testing.T) {
		t.Parallel()

		var h *config.HTTPHealthClient
		opts := factory.HealthOptions(h)
		require.Nil(t, opts)
	})

	t.Run("empty config returns empty options", func(t *testing.T) {
		t.Parallel()

		h := &config.HTTPHealthClient{}
		opts := factory.HealthOptions(h)
		require.Empty(t, opts)
	})

	t.Run("full config returns all options", func(t *testing.T) {
		t.Parallel()

		h := &config.HTTPHealthClient{
			HealthClient: config.HealthClient{
				ServiceName: "test-service",
			},
			RetryWindow:     120 * time.Second,
			RetryThreshold:  0.25,
			RetryMinSamples: 20,
			RetryBuckets:    120,
			PerHost:         true,
		}
		opts := factory.HealthOptions(h)
		// We can't easily test the exact options returned,
		// but we can verify the count
		require.Len(t, opts, 6)
	})

	t.Run("partial config returns partial options", func(t *testing.T) {
		t.Parallel()

		h := &config.HTTPHealthClient{
			HealthClient: config.HealthClient{
				ServiceName: "test-service",
			},
			RetryThreshold: 0.1,
		}
		opts := factory.HealthOptions(h)
		require.Len(t, opts, 2)
	})
}

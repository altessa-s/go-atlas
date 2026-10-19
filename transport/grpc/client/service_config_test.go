// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// White-box: the service config is private dial policy with no observable
// accessor, so the test decodes what buildServiceConfig hands to gRPC.
func TestBuildServiceConfig_HealthCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		opts            []Option
		wantHealthCheck bool
		wantRetry       bool
	}{
		{"default", nil, true, false},
		{"retry", []Option{WithRetry()}, true, true},
		{"without health check", []Option{WithoutHealthCheck()}, false, false},
		{"retry without health check", []Option{WithRetry(), WithoutHealthCheck()}, false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := &Client{options: *newOptions(tc.opts...)}
			raw := c.buildServiceConfig()

			var cfg map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(raw), &cfg), "service config must be valid JSON: %s", raw)

			_, hasHealth := cfg["healthCheckConfig"]
			require.Equal(t, tc.wantHealthCheck, hasHealth, raw)
			_, hasMethods := cfg["methodConfig"]
			require.Equal(t, tc.wantRetry, hasMethods, raw)
			_, hasLB := cfg["loadBalancingConfig"]
			require.Equal(t, tc.wantRetry, hasLB, raw)
		})
	}
}

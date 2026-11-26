// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"encoding/json"
	"testing"
	"time"

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

			health, hasHealth := cfg["healthCheckConfig"]
			require.Equal(t, tc.wantHealthCheck, hasHealth, raw)
			if hasHealth {
				require.JSONEq(t, `{"serviceName":""}`, string(health))
			}
			_, hasMethods := cfg["methodConfig"]
			require.Equal(t, tc.wantRetry, hasMethods, raw)
			_, hasLB := cfg["loadBalancingConfig"]
			require.Equal(t, tc.wantRetry || tc.wantHealthCheck, hasLB, raw)
		})
	}
}

// TestBuildServiceConfig_RetryPolicy pins the retry policy rendered for the
// default and a customized retry configuration.
func TestBuildServiceConfig_RetryPolicy(t *testing.T) {
	t.Parallel()

	type retryPolicy struct {
		MaxAttempts          int      `json:"maxAttempts"`
		InitialBackoff       string   `json:"initialBackoff"`
		MaxBackoff           string   `json:"maxBackoff"`
		BackoffMultiplier    float64  `json:"backoffMultiplier"`
		RetryableStatusCodes []string `json:"retryableStatusCodes"`
	}
	type methodConfig struct {
		Timeout     *string     `json:"timeout"`
		RetryPolicy retryPolicy `json:"retryPolicy"`
	}

	tests := []struct {
		name string
		opt  Option
		want retryPolicy
	}{
		{
			name: "default",
			opt:  WithRetry(),
			want: retryPolicy{
				MaxAttempts:          DefaultMaxRetryAttempts,
				InitialBackoff:       "1.000000000s",
				MaxBackoff:           "10.000000000s",
				BackoffMultiplier:    DefaultBackoffMultiplier,
				RetryableStatusCodes: DefaultRetryableStatusCodes,
			},
		},
		{
			name: "custom",
			opt: WithRetry(
				WithRetryMaxAttempts(3),
				WithRetryInitialBackoff(200*time.Millisecond),
				WithRetryMaxBackoff(2*time.Second),
				WithRetryBackoffMultiplier(1.5),
				WithRetryStatusCodes("UNAVAILABLE", "INTERNAL"),
			),
			want: retryPolicy{
				MaxAttempts:          3,
				InitialBackoff:       "0.200000000s",
				MaxBackoff:           "2.000000000s",
				BackoffMultiplier:    1.5,
				RetryableStatusCodes: []string{"UNAVAILABLE", "INTERNAL"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := &Client{options: *newOptions(tc.opt)}
			raw := c.buildServiceConfig()

			var cfg struct {
				MethodConfig []methodConfig `json:"methodConfig"`
			}
			require.NoError(t, json.Unmarshal([]byte(raw), &cfg), raw)
			require.Len(t, cfg.MethodConfig, 1, raw)
			require.Nil(t, cfg.MethodConfig[0].Timeout, "retries must not impose a deadline: %s", raw)
			require.Equal(t, tc.want, cfg.MethodConfig[0].RetryPolicy)
		})
	}
}

// TestNew_RetryDurationsAreValidServiceConfig dials with durations that are
// not whole seconds: grpc-go accepts only protobuf JSON durations, so a Go
// duration string such as "50ms" or "1m0s" would reject the configuration.
func TestNew_RetryDurationsAreValidServiceConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial time.Duration
		maxBack time.Duration
	}{
		{"milliseconds and minute", 50 * time.Millisecond, time.Minute},
		{"nanoseconds", 1500 * time.Nanosecond, 2*time.Second + 5*time.Millisecond},
		{"whole seconds", time.Second, 10 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, err := New(t.Context(), "passthrough:///retry-durations",
				WithInsecure(),
				WithRetry(WithRetryInitialBackoff(tc.initial), WithRetryMaxBackoff(tc.maxBack)),
			)
			require.NoError(t, err)
			require.NoError(t, c.Close(t.Context()))
		})
	}
}

func TestRetryDurationsRejectNegative(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { WithRetryInitialBackoff(-time.Second)(&retryOptions{}) })
	require.Panics(t, func() { WithRetryMaxBackoff(-time.Second)(&retryOptions{}) })
}

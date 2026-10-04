// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultHealthClient(t *testing.T) {
	t.Parallel()

	h := DefaultHealthClient()

	require.Empty(t, h.ServiceName) // ServiceName must be set explicitly
}

func TestDefaultHTTPHealthClient(t *testing.T) {
	t.Parallel()

	h := DefaultHTTPHealthClient()

	require.Empty(t, h.ServiceName) // ServiceName must be set explicitly
	require.Equal(t, DefaultHealthClientRetryWindow, h.RetryWindow)
	require.Equal(t, DefaultHealthClientRetryThreshold, h.RetryThreshold)
	require.Equal(t, DefaultHealthClientRetryMinSamples, h.RetryMinSamples)
	require.Equal(t, DefaultHealthClientRetryBuckets, h.RetryBuckets)
	require.False(t, h.PerHost)
}

func TestDefaultGRPCHealthClient(t *testing.T) {
	t.Parallel()

	h := DefaultGRPCHealthClient()

	require.Empty(t, h.ServiceName) // ServiceName must be set explicitly
	require.Equal(t, HealthClientStateMapperDefault, h.StateMapper)
	require.False(t, h.PerTarget)
}

func TestHealthClientValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		health  HealthClient
		wantErr bool
	}{
		{
			name: "valid with service name",
			health: HealthClient{
				ServiceName: "test-service",
			},
			wantErr: false,
		},
		{
			name: "invalid empty service name",
			health: HealthClient{
				ServiceName: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.health.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestHTTPHealthClientValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		health  HTTPHealthClient
		wantErr bool
	}{
		{
			name: "valid default with service name",
			health: func() HTTPHealthClient {
				h := DefaultHTTPHealthClient()
				h.ServiceName = "test-service"
				return h
			}(),
			wantErr: false,
		},
		{
			name: "valid custom",
			health: HTTPHealthClient{
				HealthClient: HealthClient{
					ServiceName: "test-service",
				},
				RetryWindow:     30 * time.Second,
				RetryThreshold:  0.5,
				RetryMinSamples: 5,
				RetryBuckets:    30,
			},
			wantErr: false,
		},
		{
			name: "invalid empty service name",
			health: HTTPHealthClient{
				HealthClient: HealthClient{
					ServiceName: "",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid retry window too small",
			health: HTTPHealthClient{
				HealthClient: HealthClient{
					ServiceName: "test-service",
				},
				RetryWindow: 500 * time.Millisecond,
			},
			wantErr: true,
		},
		{
			name: "invalid retry threshold negative",
			health: HTTPHealthClient{
				HealthClient: HealthClient{
					ServiceName: "test-service",
				},
				RetryThreshold: -0.1,
			},
			wantErr: true,
		},
		{
			name: "invalid retry threshold too high",
			health: HTTPHealthClient{
				HealthClient: HealthClient{
					ServiceName: "test-service",
				},
				RetryThreshold: 1.1,
			},
			wantErr: true,
		},
		{
			name: "zero retry min samples is valid",
			health: HTTPHealthClient{
				HealthClient: HealthClient{
					ServiceName: "test-service",
				},
				RetryMinSamples: 0,
			},
			wantErr: false,
		},
		{
			name: "zero retry buckets is valid",
			health: HTTPHealthClient{
				HealthClient: HealthClient{
					ServiceName: "test-service",
				},
				RetryBuckets: 0,
			},
			wantErr: false,
		},
		{
			name: "invalid retry buckets too many",
			health: HTTPHealthClient{
				HealthClient: HealthClient{
					ServiceName: "test-service",
				},
				RetryBuckets: 3601,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.health.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGRPCHealthClientValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		health  GRPCHealthClient
		wantErr bool
	}{
		{
			name: "valid default with service name",
			health: func() GRPCHealthClient {
				h := DefaultGRPCHealthClient()
				h.ServiceName = "test-service"
				return h
			}(),
			wantErr: false,
		},
		{
			name: "valid custom",
			health: GRPCHealthClient{
				HealthClient: HealthClient{
					ServiceName: "test-service",
				},
				StateMapper: HealthClientStateMapperStrict,
				PerTarget:   true,
			},
			wantErr: false,
		},
		{
			name: "invalid empty service name",
			health: GRPCHealthClient{
				HealthClient: HealthClient{
					ServiceName: "",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid state mapper",
			health: GRPCHealthClient{
				HealthClient: HealthClient{
					ServiceName: "test-service",
				},
				StateMapper: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.health.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

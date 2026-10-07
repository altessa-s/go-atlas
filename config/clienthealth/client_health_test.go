// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clienthealthconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultHealthClient(t *testing.T) {
	t.Parallel()

	h := Default()

	require.Empty(t, h.ServiceName) // ServiceName must be set explicitly
}

func TestDefaultHTTPHealthClient(t *testing.T) {
	t.Parallel()

	h := DefaultHTTP()

	require.Empty(t, h.ServiceName) // ServiceName must be set explicitly
	require.Equal(t, DefaultRetryWindow, h.RetryWindow)
	require.Equal(t, DefaultRetryThreshold, h.RetryThreshold)
	require.Equal(t, DefaultRetryMinSamples, h.RetryMinSamples)
	require.Equal(t, DefaultRetryBuckets, h.RetryBuckets)
	require.False(t, h.PerHost)
}

func TestDefaultGRPCHealthClient(t *testing.T) {
	t.Parallel()

	h := DefaultGRPC()

	require.Empty(t, h.ServiceName) // ServiceName must be set explicitly
	require.Equal(t, StateMapperDefault, h.StateMapper)
	require.False(t, h.PerTarget)
}

func TestHealthClientValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		health  Config
		wantErr bool
	}{
		{
			name: "valid with service name",
			health: Config{
				ServiceName: "test-service",
			},
			wantErr: false,
		},
		{
			name: "invalid empty service name",
			health: Config{
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
		health  HTTP
		wantErr bool
	}{
		{
			name: "valid default with service name",
			health: func() HTTP {
				h := DefaultHTTP()
				h.ServiceName = "test-service"
				return h
			}(),
			wantErr: false,
		},
		{
			name: "valid custom",
			health: HTTP{
				Config: Config{
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
			health: HTTP{
				Config: Config{
					ServiceName: "",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid retry window too small",
			health: HTTP{
				Config: Config{
					ServiceName: "test-service",
				},
				RetryWindow: 500 * time.Millisecond,
			},
			wantErr: true,
		},
		{
			name: "invalid retry threshold negative",
			health: HTTP{
				Config: Config{
					ServiceName: "test-service",
				},
				RetryThreshold: -0.1,
			},
			wantErr: true,
		},
		{
			name: "invalid retry threshold too high",
			health: HTTP{
				Config: Config{
					ServiceName: "test-service",
				},
				RetryThreshold: 1.1,
			},
			wantErr: true,
		},
		{
			name: "zero retry min samples is valid",
			health: HTTP{
				Config: Config{
					ServiceName: "test-service",
				},
				RetryMinSamples: 0,
			},
			wantErr: false,
		},
		{
			name: "zero retry buckets is valid",
			health: HTTP{
				Config: Config{
					ServiceName: "test-service",
				},
				RetryBuckets: 0,
			},
			wantErr: false,
		},
		{
			name: "invalid retry buckets too many",
			health: HTTP{
				Config: Config{
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
		health  GRPC
		wantErr bool
	}{
		{
			name: "valid default with service name",
			health: func() GRPC {
				h := DefaultGRPC()
				h.ServiceName = "test-service"
				return h
			}(),
			wantErr: false,
		},
		{
			name: "valid custom",
			health: GRPC{
				Config: Config{
					ServiceName: "test-service",
				},
				StateMapper: StateMapperStrict,
				PerTarget:   true,
			},
			wantErr: false,
		},
		{
			name: "invalid empty service name",
			health: GRPC{
				Config: Config{
					ServiceName: "",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid state mapper",
			health: GRPC{
				Config: Config{
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

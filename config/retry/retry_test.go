// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetry_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "ValidDefault",
			cfg:     Default(),
			wantErr: false,
		},
		{
			name: "ValidUnlimitedRetries",
			cfg: Config{
				MaxAttempts: -1,
				BaseDelay:   time.Millisecond,
				MaxDelay:    time.Second,
				Multiplier:  1.0,
			},
			wantErr: false,
		},
		{
			name: "ValidZeroAttempts",
			cfg: Config{
				MaxAttempts: 0,
				BaseDelay:   time.Millisecond,
				MaxDelay:    time.Second,
				Multiplier:  1.0,
			},
			wantErr: false,
		},
		{
			name: "InvalidMaxAttempts",
			cfg: Config{
				MaxAttempts: -2,
				BaseDelay:   time.Millisecond,
				MaxDelay:    time.Second,
				Multiplier:  1.0,
			},
			wantErr: true,
		},
		{
			name: "InvalidBaseDelay",
			cfg: Config{
				MaxAttempts: 3,
				BaseDelay:   0,
				MaxDelay:    time.Second,
				Multiplier:  1.5,
			},
			wantErr: true,
		},
		{
			name: "InvalidMaxDelay",
			cfg: Config{
				MaxAttempts: 3,
				BaseDelay:   time.Millisecond,
				MaxDelay:    0,
				Multiplier:  1.5,
			},
			wantErr: true,
		},
		{
			name: "InvalidMultiplier",
			cfg: Config{
				MaxAttempts: 3,
				BaseDelay:   time.Millisecond,
				MaxDelay:    time.Second,
				Multiplier:  0.5,
			},
			wantErr: true,
		},
		{
			name: "WithMaxElapsedTime",
			cfg: Config{
				MaxAttempts:    3,
				BaseDelay:      time.Millisecond,
				MaxDelay:       time.Second,
				Multiplier:     1.5,
				MaxElapsedTime: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "ValidJitter",
			cfg: Config{
				MaxAttempts: 3,
				BaseDelay:   time.Millisecond,
				MaxDelay:    time.Second,
				Multiplier:  1.5,
				Jitter:      0.5,
			},
			wantErr: false,
		},
		{
			name: "InvalidJitterAboveOne",
			cfg: Config{
				MaxAttempts: 3,
				BaseDelay:   time.Millisecond,
				MaxDelay:    time.Second,
				Multiplier:  1.5,
				Jitter:      1.5,
			},
			wantErr: true,
		},
		{
			name: "InvalidJitterNegative",
			cfg: Config{
				MaxAttempts: 3,
				BaseDelay:   time.Millisecond,
				MaxDelay:    time.Second,
				Multiplier:  1.5,
				Jitter:      -0.1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

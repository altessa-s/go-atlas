// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func enabledIdempotencyConfig() HttpInterIdempotencyConfig {
	cfg := DefaultHttpInterIdempotencyConfig()
	cfg.Enabled = true
	return cfg
}

func TestDefaultHttpInterIdempotencyConfig_KeyLogMode(t *testing.T) {
	t.Parallel()

	cfg := DefaultHttpInterIdempotencyConfig()
	require.Equal(t, IdempotencyKeyLogModeHashed, cfg.KeyLogMode,
		"the default must keep raw keys out of the log")
	enabled := enabledIdempotencyConfig()
	require.NoError(t, enabled.Validate())
}

func TestHttpInterIdempotency_Validate_KeyLogMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    IdempotencyKeyLogMode
		wantErr bool
	}{
		{"hashed", IdempotencyKeyLogModeHashed, false},
		{"full", IdempotencyKeyLogModeFull, false},
		{"off", IdempotencyKeyLogModeOff, false},
		{"empty", IdempotencyKeyLogMode(""), true},
		{"unknown", IdempotencyKeyLogMode("verbose"), true},
		{"wrong case", IdempotencyKeyLogMode("Hashed"), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := enabledIdempotencyConfig()
			cfg.KeyLogMode = tc.mode

			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestHttpInterIdempotency_Validate_DisabledSkipsKeyLogMode(t *testing.T) {
	t.Parallel()

	cfg := DefaultHttpInterIdempotencyConfig()
	cfg.Enabled = false
	cfg.KeyLogMode = IdempotencyKeyLogMode("verbose")

	require.NoError(t, cfg.Validate())
}

func TestAllIdempotencyKeyLogModes(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		[]IdempotencyKeyLogMode{
			IdempotencyKeyLogModeHashed,
			IdempotencyKeyLogModeFull,
			IdempotencyKeyLogModeOff,
		},
		AllIdempotencyKeyLogModes())
}

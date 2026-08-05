// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

func TestStorageNATSConfig_Validate(t *testing.T) {
	cases := []struct {
		name      string
		cfg       config.StorageNATSConfig
		wantValid bool
	}{
		{
			name:      "Valid",
			cfg:       config.StorageNATSConfig{Replicas: 3},
			wantValid: true,
		},
		{
			name:      "ZeroReplicas",
			cfg:       config.StorageNATSConfig{Replicas: 0},
			wantValid: false, // Required is what rejects it — Min(1) alone skips the zero value
		},
		{
			name:      "TooManyReplicas",
			cfg:       config.StorageNATSConfig{Replicas: 4}, // Max is 3
			wantValid: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantValid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestStorageRedisConfig_Validate(t *testing.T) {
	validPrefix := "valid_prefix"
	invalidPrefix := make([]byte, 65) // Max 64
	for i := range invalidPrefix {
		invalidPrefix[i] = 'a'
	}

	cases := []struct {
		name      string
		cfg       config.StorageRedisConfig
		wantValid bool
	}{
		{
			name:      "Valid",
			cfg:       config.StorageRedisConfig{KeysPrefix: validPrefix},
			wantValid: true,
		},
		{
			name:      "EmptyPrefix",
			cfg:       config.StorageRedisConfig{KeysPrefix: ""},
			wantValid: true,
		},
		{
			name:      "TooLongPrefix",
			cfg:       config.StorageRedisConfig{KeysPrefix: string(invalidPrefix)},
			wantValid: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantValid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestCacheStorageConfig_Validate(t *testing.T) {
	cases := []struct {
		name      string
		cfg       config.CacheStorageConfig
		wantValid bool
	}{
		{
			name: "Memory_Valid",
			cfg: config.CacheStorageConfig{
				Type:   config.CacheStorageTypeMemory,
				Memory: &config.StorageMemoryConfig{CleanupSchedule: "@every 5m"},
			},
			wantValid: true,
		},
		{
			name: "Memory_MissingConfig",
			cfg: config.CacheStorageConfig{
				Type: config.CacheStorageTypeMemory,
			},
			wantValid: true, // NilOrNotEmpty allows nil (field is optional)
		},
		{
			name: "Redis_Valid",
			cfg: config.CacheStorageConfig{
				Type:  config.CacheStorageTypeRedis,
				Redis: &config.StorageRedisConfig{},
			},
			wantValid: true,
		},
		{
			name: "InvalidType",
			cfg: config.CacheStorageConfig{
				Type: "invalid",
			},
			wantValid: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantValid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

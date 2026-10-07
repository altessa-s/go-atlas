// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
)

func TestStorageNATSConfig_Validate(t *testing.T) {
	cases := []struct {
		name      string
		cfg       storageconfig.NATSConfig
		wantValid bool
	}{
		{
			name:      "Valid",
			cfg:       storageconfig.NATSConfig{Replicas: 3},
			wantValid: true,
		},
		{
			name:      "ZeroReplicas",
			cfg:       storageconfig.NATSConfig{Replicas: 0},
			wantValid: false, // Required is what rejects it — Min(1) alone skips the zero value
		},
		{
			name:      "TooManyReplicas",
			cfg:       storageconfig.NATSConfig{Replicas: 4}, // Max is 3
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
		cfg       storageconfig.RedisConfig
		wantValid bool
	}{
		{
			name:      "Valid",
			cfg:       storageconfig.RedisConfig{KeysPrefix: validPrefix},
			wantValid: true,
		},
		{
			name:      "EmptyPrefix",
			cfg:       storageconfig.RedisConfig{KeysPrefix: ""},
			wantValid: true,
		},
		{
			name:      "TooLongPrefix",
			cfg:       storageconfig.RedisConfig{KeysPrefix: string(invalidPrefix)},
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
		cfg       storageconfig.CacheStorageConfig
		wantValid bool
	}{
		{
			name: "Memory_Valid",
			cfg: storageconfig.CacheStorageConfig{
				Type:   storageconfig.CacheStorageTypeMemory,
				Memory: &storageconfig.MemoryConfig{CleanupSchedule: "@every 5m"},
			},
			wantValid: true,
		},
		{
			name: "Memory_MissingConfig",
			cfg: storageconfig.CacheStorageConfig{
				Type: storageconfig.CacheStorageTypeMemory,
			},
			wantValid: true, // NilOrNotEmpty allows nil (field is optional)
		},
		{
			name: "Redis_Valid",
			cfg: storageconfig.CacheStorageConfig{
				Type:  storageconfig.CacheStorageTypeRedis,
				Redis: &storageconfig.RedisConfig{},
			},
			wantValid: true,
		},
		{
			name: "InvalidType",
			cfg: storageconfig.CacheStorageConfig{
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

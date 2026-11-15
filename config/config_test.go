// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
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
		{
			// The cache factories have no NATS storage, so validation must
			// reject the type instead of the build failing at runtime.
			name: "NATS_Rejected",
			cfg: storageconfig.CacheStorageConfig{
				Type: "nats",
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

func TestBackendConfig_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		cfg       storageconfig.BackendConfig
		wantValid bool
	}{
		{name: "Memory", cfg: storageconfig.BackendConfig{Type: storageconfig.BackendTypeMemory}, wantValid: true},
		{
			name:      "Redis",
			cfg:       storageconfig.BackendConfig{Type: storageconfig.BackendTypeRedis, Redis: &storageconfig.RedisConfig{}},
			wantValid: true,
		},
		{
			name:      "NATS",
			cfg:       storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS, NATS: &storageconfig.NATSConfig{Replicas: 1}},
			wantValid: true,
		},
		{name: "InvalidType", cfg: storageconfig.BackendConfig{Type: "invalid"}, wantValid: false},
		{name: "MissingType", cfg: storageconfig.BackendConfig{}, wantValid: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			if tc.wantValid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestLimiters_AcceptNATSStorage pins that the rate limiters keep the NATS
// storage backend the cache storage config rejects.
func TestLimiters_AcceptNATSStorage(t *testing.T) {
	t.Parallel()

	storage := &storageconfig.BackendConfig{
		Type: storageconfig.BackendTypeNATS,
		NATS: &storageconfig.NATSConfig{Bucket: "limits", Replicas: 1},
	}

	budget := limiterconfig.DefaultBudget()
	budget.Limit = 10
	budget.Storage = storage
	require.NoError(t, budget.Validate())

	bucket := limiterconfig.DefaultTokenBucket()
	bucket.Storage = storage
	bucket.Rules = &limiterconfig.TokenBucketRules{
		Default: &limiterconfig.TokenBucketDefaultRule{Limit: 10, Period: time.Minute},
	}
	require.NoError(t, bucket.Validate())
}

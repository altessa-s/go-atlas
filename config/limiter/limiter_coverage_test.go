// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterconfig_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"
	"github.com/altessa-s/go-nomos/loader/backend/yaml3"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
)

func memoryStorage() *limiterconfig.StorageConfig {
	return &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeMemory, Memory: &storageconfig.MemoryConfig{}}
}

func TestStorageConfig_NormalizeAndValidate(t *testing.T) {
	t.Parallel()

	t.Run("normalize allocates selected sub-config", func(t *testing.T) {
		t.Parallel()
		mem := limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeMemory}
		mem.Normalize()
		require.NotNil(t, mem.Memory)
		require.Nil(t, mem.Redis)

		redis := limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis}
		redis.Normalize()
		require.NotNil(t, redis.Redis)
		require.Nil(t, redis.Memory)

		unknown := limiterconfig.StorageConfig{Type: "nats"}
		unknown.Normalize()
		require.Nil(t, unknown.Memory)
		require.Nil(t, unknown.Redis)
	})

	t.Run("validate accepts memory, redis, and registered custom types", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, (&limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeMemory, Memory: &storageconfig.MemoryConfig{}}).Validate())
		require.NoError(t, (&limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis, Redis: &storageconfig.RedisConfig{}}).Validate())
		require.Error(t, (&limiterconfig.StorageConfig{}).Validate())
		require.Error(t, (&limiterconfig.StorageConfig{Type: "nats"}).Validate())

		custom := limiterconfig.StorageConfig{Type: "custom"}
		custom.SetRegisteredTypes([]limiterconfig.StorageType{"custom"})
		require.NoError(t, custom.Validate())
	})

	t.Run("loader normalizes nested Storage without top-level Normalize", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "limiter.yaml")
		content := "budgetLimiter:\n  limit: 10\n  storage:\n    type: redis\ntokenBucketLimiter:\n  storage:\n    type: memory\n  rules:\n    default:\n      limit: 5\n      period: \"1m\"\n"
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		var root struct {
			Budget      limiterconfig.Budget      `yaml:"budgetLimiter"`
			TokenBucket limiterconfig.TokenBucket `yaml:"tokenBucketLimiter"`
		}
		_, err := loader.New(&yaml3.Backend{}, loader.WithPath(path), loader.WithSkipEnv()).Load(&root)
		require.NoError(t, err)
		require.NotNil(t, root.Budget.Storage)
		require.NotNil(t, root.Budget.Storage.Redis)
		require.NoError(t, root.Budget.Validate())
		require.NotNil(t, root.TokenBucket.Storage)
		require.NotNil(t, root.TokenBucket.Storage.Memory)
		require.NoError(t, root.TokenBucket.Validate())
	})
}

func TestDefaultRequestRate(t *testing.T) {
	t.Parallel()

	got := limiterconfig.DefaultRequestRate()
	require.Equal(t, 2500, got.Rate)
	require.Equal(t, 24*time.Hour, got.Period)
	require.Equal(t, 100, got.Burst)
	require.NoError(t, got.Validate())
}

func TestRequestRate_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     limiterconfig.RequestRate
		wantErr bool
	}{
		{name: "defaults are valid", cfg: limiterconfig.DefaultRequestRate()},
		{name: "zero value fails on every required field", cfg: limiterconfig.RequestRate{}, wantErr: true},
		{name: "zero rate fails", cfg: limiterconfig.RequestRate{Rate: 0, Period: time.Hour, Burst: 10}, wantErr: true},
		{name: "zero period fails duration rule", cfg: limiterconfig.RequestRate{Rate: 10, Period: 0, Burst: 10}, wantErr: true},
		{name: "zero burst fails", cfg: limiterconfig.RequestRate{Rate: 10, Period: time.Hour, Burst: 0}, wantErr: true},
		{name: "fully specified passes", cfg: limiterconfig.RequestRate{Rate: 10, Period: time.Minute, Burst: 5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultTokenBucketDefaultRule(t *testing.T) {
	t.Parallel()

	got := limiterconfig.DefaultTokenBucketDefaultRule()
	require.Equal(t, int64(1000), got.Limit)
	require.Equal(t, time.Hour, got.Period)
	require.NoError(t, got.Validate())
}

func TestTokenBucketDefaultRule_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     limiterconfig.TokenBucketDefaultRule
		wantErr bool
	}{
		{name: "defaults are valid", cfg: limiterconfig.DefaultTokenBucketDefaultRule()},
		{name: "zero value fails", cfg: limiterconfig.TokenBucketDefaultRule{}, wantErr: true},
		{name: "negative limit fails", cfg: limiterconfig.TokenBucketDefaultRule{Limit: -1, Period: time.Minute}, wantErr: true},
		{name: "period below one second fails", cfg: limiterconfig.TokenBucketDefaultRule{Limit: 1, Period: time.Millisecond}, wantErr: true},
		{name: "valid rule passes", cfg: limiterconfig.TokenBucketDefaultRule{Limit: 1, Period: time.Second}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestTokenBucketTargetRule_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     limiterconfig.TokenBucketTargetRule
		wantErr bool
	}{
		{name: "zero value fails", cfg: limiterconfig.TokenBucketTargetRule{}, wantErr: true},
		{name: "missing target fails", cfg: limiterconfig.TokenBucketTargetRule{Limit: 1, Period: time.Second}, wantErr: true},
		{name: "missing limit fails", cfg: limiterconfig.TokenBucketTargetRule{Target: "10.0.0.1", Period: time.Second}, wantErr: true},
		{name: "missing period fails", cfg: limiterconfig.TokenBucketTargetRule{Target: "10.0.0.1", Limit: 1}, wantErr: true},
		{
			name: "fully specified passes",
			cfg:  limiterconfig.TokenBucketTargetRule{Target: "10.0.0.1", Limit: 1, Period: time.Second},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestTokenBucketRules_Validate(t *testing.T) {
	t.Parallel()

	validDefault := &limiterconfig.TokenBucketDefaultRule{Limit: 1, Period: time.Second}
	validTarget := limiterconfig.TokenBucketTargetRule{Target: "10.0.0.1", Limit: 1, Period: time.Second}
	invalidTarget := limiterconfig.TokenBucketTargetRule{Target: "", Limit: 1, Period: time.Second}

	tests := []struct {
		name    string
		cfg     limiterconfig.TokenBucketRules
		wantErr bool
	}{
		{name: "missing default fails", cfg: limiterconfig.TokenBucketRules{}, wantErr: true},
		{name: "default only passes", cfg: limiterconfig.TokenBucketRules{Default: validDefault}},
		{
			name: "default with valid target passes",
			cfg:  limiterconfig.TokenBucketRules{Default: validDefault, Targets: []limiterconfig.TokenBucketTargetRule{validTarget}},
		},
		{
			name:    "invalid target propagates the target's own error",
			cfg:     limiterconfig.TokenBucketRules{Default: validDefault, Targets: []limiterconfig.TokenBucketTargetRule{invalidTarget}},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultTokenBucket(t *testing.T) {
	t.Parallel()

	got := limiterconfig.DefaultTokenBucket()
	require.Equal(t, 1000, got.IpCacheSize)
	require.Nil(t, got.Storage, "Storage is required and intentionally left unset by the default")
	require.Nil(t, got.Rules, "Rules is required and intentionally left unset by the default")
	// The default alone is incomplete: Storage/Rules are required.
	require.Error(t, got.Validate())
}

func TestTokenBucket_Validate(t *testing.T) {
	t.Parallel()

	validRules := &limiterconfig.TokenBucketRules{Default: &limiterconfig.TokenBucketDefaultRule{Limit: 1, Period: time.Second}}

	tests := []struct {
		name    string
		cfg     limiterconfig.TokenBucket
		wantErr bool
	}{
		{name: "zero value fails (missing storage and rules)", cfg: limiterconfig.TokenBucket{}, wantErr: true},
		{name: "missing storage fails", cfg: limiterconfig.TokenBucket{Rules: validRules}, wantErr: true},
		{name: "missing rules fails", cfg: limiterconfig.TokenBucket{Storage: memoryStorage()}, wantErr: true},
		{name: "negative IP cache size fails", cfg: limiterconfig.TokenBucket{IpCacheSize: -1, Storage: memoryStorage(), Rules: validRules}, wantErr: true},
		{
			name: "fully specified passes",
			cfg:  limiterconfig.TokenBucket{IpCacheSize: 1000, Storage: memoryStorage(), Rules: validRules},
		},
		{
			name: "nats storage fails validation",
			cfg: limiterconfig.TokenBucket{
				Storage: &limiterconfig.StorageConfig{Type: "nats"},
				Rules:   validRules,
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultBudget(t *testing.T) {
	t.Parallel()

	got := limiterconfig.DefaultBudget()
	require.Equal(t, 24*time.Hour, got.Period)
	require.Nil(t, got.Storage, "Storage is required and intentionally left unset by the default")
	// The default alone is incomplete: Limit and Storage are required.
	require.Error(t, got.Validate())
}

func TestBudget_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     limiterconfig.Budget
		wantErr bool
	}{
		{name: "zero value fails", cfg: limiterconfig.Budget{}, wantErr: true},
		{name: "missing storage fails", cfg: limiterconfig.Budget{Limit: 1, Period: time.Hour}, wantErr: true},
		{name: "zero limit fails", cfg: limiterconfig.Budget{Limit: 0, Period: time.Hour, Storage: memoryStorage()}, wantErr: true},
		{name: "period below one second fails", cfg: limiterconfig.Budget{Limit: 1, Period: time.Millisecond, Storage: memoryStorage()}, wantErr: true},
		{name: "fully specified passes", cfg: limiterconfig.Budget{Limit: 1, Period: time.Hour, Storage: memoryStorage()}},
		{
			name: "nats storage fails validation",
			cfg: limiterconfig.Budget{
				Limit: 1, Period: time.Hour,
				Storage: &limiterconfig.StorageConfig{Type: "nats"},
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

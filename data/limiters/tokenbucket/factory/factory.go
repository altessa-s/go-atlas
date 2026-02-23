// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	memorystorage "github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages/memory"
	natsstorage "github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages/nats"
	redisstorage "github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages/redis"
)

const (
	// DefaultLimit is the default rate limit when no configuration is provided.
	DefaultLimit = 1000
	// DefaultPeriod is the default time period for rate limiting.
	DefaultPeriod = time.Hour
)

// Factory creates rate limiters with configured storage backends.
type Factory struct {
	corefactory.Base
	redisClient redis.UniversalClient
	jetstream   jetstream.JetStream
	opts        *options
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:        corefactory.NewBase(cfg.logger),
		redisClient: cfg.redisClient,
		jetstream:   cfg.jetstream,
		opts:        cfg,
	}
}

// createLimiter creates a RuleLimiter with the given config and storage.
func (f *Factory) createLimiter(
	cfg *tokenbucket.RateLimitConfig,
	storage storages.Storage,
	opts ...tokenbucket.Option,
) *tokenbucket.RuleLimiter {
	return tokenbucket.New(cfg, storage, f.applyDefaults(opts)...)
}

// CreateLimiterFromConfig creates a RuleLimiter from configuration.
func (f *Factory) CreateLimiterFromConfig(
	cfg *config.Limiter,
	storage storages.Storage,
	opts ...tokenbucket.Option,
) (*tokenbucket.RuleLimiter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	rateLimitConfig := convertConfig(cfg)
	opts = append(opts, tokenbucket.WithIPCacheSize(cfg.IpCacheSize))

	return f.createLimiter(rateLimitConfig, storage, opts...), nil
}

// CreateStorageFromConfig creates a storage backend based on configuration.
func (f *Factory) CreateStorageFromConfig(cfg *config.CacheStorageConfig) (storages.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch cfg.Type {
	case config.CacheStorageTypeMemory:
		return f.CreateMemoryStorageFromConfig(cfg.Memory)
	case config.CacheStorageTypeRedis:
		if err := f.RequireDependency(f.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return f.CreateRedisStorageFromConfig(cfg.Redis)
	case config.CacheStorageTypeNats:
		if err := f.RequireDependency(f.jetstream, "jetstream"); err != nil {
			return nil, err
		}
		return f.CreateNatsStorageFromConfig(cfg.Nats)
	default:
		return nil, f.Errorf("unsupported storage type: %s", cfg.Type)
	}
}

// CreateMemoryStorageFromConfig creates an in-memory storage from configuration.
func (f *Factory) CreateMemoryStorageFromConfig(cfg *config.StorageMemoryConfig) (*memorystorage.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return memorystorage.New(
		memorystorage.WithCleanupSchedule(cfg.CleanupSchedule),
		memorystorage.WithScheduler(f.opts.scheduler),
	), nil
}

// CreateRedisStorageFromConfig creates a Redis storage from configuration.
func (f *Factory) CreateRedisStorageFromConfig(cfg *config.StorageRedisConfig) (*redisstorage.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return redisstorage.New(f.redisClient, redisstorage.WithKeyPrefix(cfg.KeysPrefix)), nil
}

// CreateNatsStorageFromConfig creates a NATS storage from configuration.
func (f *Factory) CreateNatsStorageFromConfig(cfg *config.StorageNATSConfig) (*natsstorage.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return natsstorage.New(f.jetstream,
		natsstorage.WithBucket(cfg.Bucket),
		natsstorage.WithReplicas(cfg.Replicas),
	)
}

// applyDefaults prepends factory defaults to limiter options.
func (f *Factory) applyDefaults(opts []tokenbucket.Option) []tokenbucket.Option {
	defaults := []tokenbucket.Option{
		tokenbucket.WithLogger(f.Logger()),
		tokenbucket.WithExtractClientIPAddress(tokenbucket.ExtractClientIp),
		tokenbucket.WithExtractToken(tokenbucket.ExtractAuthToken),
	}
	return append(defaults, opts...)
}

// convertConfig converts config.Limiter to tokenbucket.RateLimitConfig.
func convertConfig(cfg *config.Limiter) *tokenbucket.RateLimitConfig {
	if cfg == nil || cfg.Rules == nil {
		return &tokenbucket.RateLimitConfig{
			Default: tokenbucket.RateLimitSettings{
				Limit:  DefaultLimit,
				Period: DefaultPeriod,
			},
		}
	}

	result := &tokenbucket.RateLimitConfig{
		Rules: make([]*tokenbucket.RateLimitRule, 0, len(cfg.Rules.Targets)),
	}

	if cfg.Rules.Default != nil {
		result.Default = tokenbucket.RateLimitSettings{
			Limit:  cfg.Rules.Default.Limit,
			Period: cfg.Rules.Default.Period,
		}
	}

	for _, rule := range cfg.Rules.Targets {
		result.Rules = append(result.Rules, &tokenbucket.RateLimitRule{
			Target: rule.Target,
			RateLimitSettings: &tokenbucket.RateLimitSettings{
				Limit:  rule.Limit,
				Period: rule.Period,
			},
		})
	}

	return result
}

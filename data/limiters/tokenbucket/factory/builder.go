// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
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

// LimiterBuilder assembles a [tokenbucket.RuleLimiter] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [LimiterBuilder.Build] time.
// The builder is not safe for concurrent use.
type LimiterBuilder struct {
	corefactory.Base
	cfg  *config.Limiter
	errs []error

	// Dependencies
	redisClient redis.UniversalClient
	jetstream   jetstream.JetStream
	scheduler   corescheduler.TaskRegistrar
	collector   metrics.Collector
}

// New creates a [LimiterBuilder] for the given limiter config.
// Config can be nil — the error surfaces at [LimiterBuilder.Build] time.
func New(cfg *config.Limiter) *LimiterBuilder {
	return &LimiterBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the rate limiter. It creates the storage from config internally,
// then creates the limiter. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *LimiterBuilder) Build() (*tokenbucket.RuleLimiter, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storage, err := b.createStorage()
	if err != nil {
		return nil, err
	}

	rateLimitConfig := convertConfig(b.cfg)
	opts := b.applyDefaults([]tokenbucket.Option{
		tokenbucket.WithIPCacheSize(b.cfg.IpCacheSize),
	})

	return tokenbucket.New(rateLimitConfig, storage, opts...)
}

// createStorage creates a storage backend based on configuration.
func (b *LimiterBuilder) createStorage() (storages.Storage, error) {
	if b.cfg.Storage == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch b.cfg.Storage.Type {
	case config.CacheStorageTypeMemory:
		return b.createMemoryStorage()
	case config.CacheStorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return b.createRedisStorage()
	case config.CacheStorageTypeNats:
		if err := b.RequireDependency(b.jetstream, "jetstream"); err != nil {
			return nil, err
		}
		return b.createNatsStorage()
	default:
		return nil, b.Errorf("unsupported storage type: %s", b.cfg.Storage.Type)
	}
}

// createMemoryStorage creates an in-memory storage from configuration.
func (b *LimiterBuilder) createMemoryStorage() (*memorystorage.Provider, error) {
	if b.cfg.Storage.Memory == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return memorystorage.New(
		memorystorage.WithCleanupSchedule(b.cfg.Storage.Memory.CleanupSchedule),
		memorystorage.WithScheduler(b.scheduler),
	), nil
}

// createRedisStorage creates a Redis storage from configuration.
func (b *LimiterBuilder) createRedisStorage() (*redisstorage.Provider, error) {
	if b.cfg.Storage.Redis == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return redisstorage.New(b.redisClient, redisstorage.WithKeyPrefix(b.cfg.Storage.Redis.KeysPrefix)), nil
}

// createNatsStorage creates a NATS storage from configuration.
func (b *LimiterBuilder) createNatsStorage() (*natsstorage.Provider, error) {
	if b.cfg.Storage.Nats == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return natsstorage.New(b.jetstream,
		natsstorage.WithBucket(b.cfg.Storage.Nats.Bucket),
		natsstorage.WithReplicas(b.cfg.Storage.Nats.Replicas),
	)
}

// applyDefaults prepends factory defaults to limiter options.
func (b *LimiterBuilder) applyDefaults(opts []tokenbucket.Option) []tokenbucket.Option {
	defaults := []tokenbucket.Option{
		tokenbucket.WithLogger(b.Logger()),
		tokenbucket.WithCollector(b.collector),
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

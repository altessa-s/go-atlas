// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/coord/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	memorystorage "github.com/altessa-s/go-atlas/coord/limiters/storages/memory"
	storagesfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
)

const (
	// DefaultLimit is the default rate limit when no configuration is provided.
	DefaultLimit = 1000
	// DefaultPeriod is the default time period for rate limiting.
	DefaultPeriod = time.Hour
)

// TokenBucketLimiterBuilder assembles a [tokenbucket.RuleLimiter] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [TokenBucketLimiterBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds only the in-memory storage. The Redis and NATS ones
// register their [storagesfactory.StorageFunc] with
// [TokenBucketLimiterBuilder.UseStorage] from
// factory/coord/limiters/storages/redis and factory/coord/limiters/storages/nats;
// a configured type without one fails at Build naming the package to add.
type TokenBucketLimiterBuilder struct {
	corefactory.Base
	cfg  *limiterconfig.TokenBucket
	errs []error

	// Dependencies
	scheduler     schedtask.Registrar
	collector     metrics.Meter
	clientService tokenbucket.ClientService

	storages map[storageconfig.BackendType]storagesfactory.StorageFunc
}

// New creates a [TokenBucketLimiterBuilder] for the given limiter config.
// Config can be nil — the error surfaces at [TokenBucketLimiterBuilder.Build] time.
func New(cfg *limiterconfig.TokenBucket) *TokenBucketLimiterBuilder {
	return &TokenBucketLimiterBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the rate limiter. It creates the storage from config internally,
// then creates the limiter. Errors from fluent methods are accumulated
// and reported here via [errors.Join]. ctx bounds storage provisioning
// (the NATS KeyValue bucket is created on it).
func (b *TokenBucketLimiterBuilder) Build(ctx context.Context) (*tokenbucket.RuleLimiter, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	defaults, rules := convertConfig(b.cfg)
	storage, err := b.createStorage(ctx, defaults, rules)
	if err != nil {
		return nil, err
	}

	opts := b.applyDefaults([]tokenbucket.Option{
		tokenbucket.WithIPCacheSize(b.cfg.IpCacheSize),
		tokenbucket.WithRules(rules...),
	})
	opts = coreslices.AppendIf(opts, b.clientService != nil, tokenbucket.WithClientService(b.clientService))

	return tokenbucket.New(defaults, storage, opts...)
}

// createStorage creates a storage backend based on configuration.
func (b *TokenBucketLimiterBuilder) createStorage(
	ctx context.Context, defaults tokenbucket.RateLimitSettings, rules []*tokenbucket.RateLimitRule,
) (storages.Storage, error) {
	if b.cfg.Storage == nil {
		return nil, corefactory.ErrConfigRequired
	}

	if b.cfg.Storage.Type == storageconfig.BackendTypeMemory {
		return b.createMemoryStorage() //nolint:contextcheck // the memory cleanup task is process-scoped, not tied to the startup ctx
	}
	var longestPeriod time.Duration
	if b.cfg.Storage.Type == storageconfig.BackendTypeNATS {
		if !defaults.IsUnlimited() {
			longestPeriod = defaults.Period
		}
		for _, rule := range rules {
			if !rule.IsUnlimited() {
				longestPeriod = max(longestPeriod, rule.Period)
			}
		}
	}
	return storagesfactory.BuildRegistered(ctx, b.cfg.Storage, b.cfg.NATSContention, b.storages, "token-bucket", longestPeriod)
}

// createMemoryStorage creates an in-memory storage from configuration.
func (b *TokenBucketLimiterBuilder) createMemoryStorage() (*memorystorage.Provider, error) {
	if b.cfg.Storage.Memory == nil {
		return nil, corefactory.ErrConfigRequired
	}

	opts := []memorystorage.Option{
		memorystorage.WithCleanupSchedule(b.cfg.Storage.Memory.CleanupSchedule),
		memorystorage.WithScheduler(b.scheduler),
	}
	opts = coreslices.AppendIf(opts, b.cfg.InsecureEvictLRU, memorystorage.WithInsecureEvictLRU())
	return memorystorage.New(opts...), nil
}

// applyDefaults prepends factory defaults to limiter options.
func (b *TokenBucketLimiterBuilder) applyDefaults(opts []tokenbucket.Option) []tokenbucket.Option {
	defaults := make([]tokenbucket.Option, 0, 4+len(opts))
	defaults = append(defaults,
		tokenbucket.WithLogger(b.Logger()),
		tokenbucket.WithMeter(b.collector),
		tokenbucket.WithExtractClientIPAddress(limiters.ClientIPFromContext),
		tokenbucket.WithExtractToken(limiters.AuthTokenFromContext),
	)
	return append(defaults, opts...)
}

// convertConfig maps limiterconfig.TokenBucket to the limiter's default
// settings and its IP/CIDR rules.
func convertConfig(cfg *limiterconfig.TokenBucket) (tokenbucket.RateLimitSettings, []*tokenbucket.RateLimitRule) {
	if cfg == nil || cfg.Rules == nil {
		return tokenbucket.RateLimitSettings{Limit: DefaultLimit, Period: DefaultPeriod}, nil
	}

	var defaults tokenbucket.RateLimitSettings
	if cfg.Rules.Default != nil {
		defaults = tokenbucket.RateLimitSettings{
			Limit:  cfg.Rules.Default.Limit,
			Period: cfg.Rules.Default.Period,
		}
	}

	rules := make([]*tokenbucket.RateLimitRule, 0, len(cfg.Rules.Targets))
	for _, rule := range cfg.Rules.Targets {
		rules = append(rules, &tokenbucket.RateLimitRule{
			Target: rule.Target,
			RateLimitSettings: &tokenbucket.RateLimitSettings{
				Limit:  rule.Limit,
				Period: rule.Period,
			},
		})
	}

	return defaults, rules
}

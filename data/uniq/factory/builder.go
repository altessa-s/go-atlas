// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/uniq"
	"github.com/altessa-s/go-atlas/data/uniq/providers"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	natsprovider "github.com/altessa-s/go-atlas/data/uniq/providers/nats"
	noopprovider "github.com/altessa-s/go-atlas/data/uniq/providers/noop"
	redisprovider "github.com/altessa-s/go-atlas/data/uniq/providers/redis"
)

// UniqBuilder assembles a [uniq.Uniq] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [UniqBuilder.Build] time.
// The builder is not safe for concurrent use.
type UniqBuilder struct {
	corefactory.Base
	cfg  *config.CacheStorageConfig
	errs []error

	// Dependencies
	redisClient       redis.UniversalClient
	natsConn          *nats.Conn
	collector         metrics.Collector
	healthCoordinator *health.Coordinator
	healthServiceName string
}

// New creates a [UniqBuilder] for the given cache storage config.
// Config can be nil — the builder returns a Uniq with a no-op provider if nil.
func New(cfg *config.CacheStorageConfig) *UniqBuilder {
	return &UniqBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the unique value manager. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *UniqBuilder) Build() (*uniq.Uniq, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	provider, err := b.createProvider()
	if err != nil {
		return nil, err
	}

	return uniq.New(provider, b.applyDefaults()...), nil
}

// applyDefaults returns builder-supplied options to pass to [uniq.New].
func (b *UniqBuilder) applyDefaults() []uniq.Option {
	opts := []uniq.Option{uniq.WithLogger(b.Logger())}
	opts = coreslices.AppendIf(opts, b.collector != nil,
		uniq.WithCollector(b.collector))
	opts = coreslices.AppendIf(opts, b.healthCoordinator != nil,
		uniq.WithHealthCoordinator(b.healthCoordinator))
	opts = coreslices.AppendIf(opts, b.healthServiceName != "",
		uniq.WithHealthServiceName(b.healthServiceName))
	return opts
}

// createProvider creates a provider based on configuration.
// If cfg is nil, returns a no-op provider.
func (b *UniqBuilder) createProvider() (providers.Provider, error) {
	if b.cfg == nil {
		return noopprovider.New(), nil
	}

	switch b.cfg.Type {
	case config.CacheStorageTypeMemory:
		return noopprovider.New(), nil
	case config.CacheStorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return b.createRedisProvider()
	case config.CacheStorageTypeNats:
		if err := b.RequireDependency(b.natsConn, "nats connection"); err != nil {
			return nil, err
		}
		return b.createNatsProvider()
	default:
		return nil, b.Errorf("unsupported storage type: %s", b.cfg.Type)
	}
}

// createRedisProvider creates a Redis provider from configuration.
func (b *UniqBuilder) createRedisProvider() (*redisprovider.Provider, error) {
	if b.cfg.Redis == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	return redisprovider.New(b.redisClient, redisprovider.WithPrefix(b.cfg.Redis.KeysPrefix)), nil
}

// createNatsProvider creates a NATS provider from configuration.
func (b *UniqBuilder) createNatsProvider() (*natsprovider.Provider, error) {
	if b.cfg.Nats == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	return natsprovider.New(b.natsConn, natsprovider.WithBucket(b.cfg.Nats.Bucket))
}

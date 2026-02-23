// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/uniq"
	"github.com/altessa-s/go-atlas/data/uniq/providers"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	natsprovider "github.com/altessa-s/go-atlas/data/uniq/providers/nats"
	noopprovider "github.com/altessa-s/go-atlas/data/uniq/providers/noop"
	redisprovider "github.com/altessa-s/go-atlas/data/uniq/providers/redis"
)

// Factory creates unique value managers with configured backends.
type Factory struct {
	corefactory.Base
	redisClient redis.UniversalClient
	natsConn    *nats.Conn
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:        corefactory.NewBase(cfg.logger),
		redisClient: cfg.redisClient,
		natsConn:    cfg.natsConn,
	}
}

// createUniq creates a Uniq with the given provider.
func (f *Factory) createUniq(provider providers.Provider) *uniq.Uniq {
	return uniq.New(provider)
}

// CreateNoopProvider creates a no-op provider for testing.
func (f *Factory) CreateNoopProvider() *noopprovider.Provider {
	return noopprovider.New()
}

// CreateRedisProviderFromConfig creates a Redis provider from configuration.
func (f *Factory) CreateRedisProviderFromConfig(cfg *config.StorageRedisConfig) (*redisprovider.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return redisprovider.New(f.redisClient, redisprovider.WithPrefix(cfg.KeysPrefix)), nil
}

// CreateNatsProviderFromConfig creates a NATS provider from configuration.
func (f *Factory) CreateNatsProviderFromConfig(cfg *config.StorageNATSConfig) (*natsprovider.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return natsprovider.New(f.natsConn, natsprovider.WithBucket(cfg.Bucket))
}

// CreateProviderFromConfig creates a provider based on configuration.
// If cfg is nil, returns a no-op provider.
func (f *Factory) CreateProviderFromConfig(cfg *config.CacheStorageConfig) (providers.Provider, error) {
	if cfg == nil {
		return f.CreateNoopProvider(), nil
	}

	switch cfg.Type {
	case config.CacheStorageTypeMemory:
		return f.CreateNoopProvider(), nil
	case config.CacheStorageTypeRedis:
		if err := f.RequireDependency(f.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return f.CreateRedisProviderFromConfig(cfg.Redis)
	case config.CacheStorageTypeNats:
		if err := f.RequireDependency(f.natsConn, "nats connection"); err != nil {
			return nil, err
		}
		return f.CreateNatsProviderFromConfig(cfg.Nats)
	default:
		return nil, f.Errorf("unsupported storage type: %s", cfg.Type)
	}
}

// CreateUniqFromConfig creates a Uniq from configuration.
// If cfg is nil, returns a Uniq with a no-op provider.
func (f *Factory) CreateUniqFromConfig(cfg *config.CacheStorageConfig) (*uniq.Uniq, error) {
	provider, err := f.CreateProviderFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	return f.createUniq(provider), nil
}

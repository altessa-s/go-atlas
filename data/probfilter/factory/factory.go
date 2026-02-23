// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of probabilistic filters.
package factory

import (
	"cmp"
	"fmt"
	"io"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	bloomstorages "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages"
	bloommemory "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
	bloomredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
	cuckoostorages "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages"
	cuckoomemory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
	cuckoeredis "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/redis"
)

const (
	// defaultBloomFPR is the default false positive rate for Bloom filters.
	defaultBloomFPR = 0.01
)

// Factory creates filters from configuration.
type Factory struct {
	corefactory.Base
	redisClient redis.UniversalClient
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:        corefactory.NewBase(cfg.logger),
		redisClient: cfg.redisClient,
	}
}

// CreateFilterFromConfig creates a filter from configuration.
func (f *Factory) CreateFilterFromConfig(
	name string,
	cfg *config.ProbabilisticFilterConfig,
	defaults *config.ProbabilisticFilterDefaults,
) (probfilter.Filter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if defaults == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch cfg.Type {
	case config.ProbabilisticFilterTypeBloom:
		return f.CreateBloomFilterFromConfig(name, cfg.Bloom, defaults.Bloom)
	case config.ProbabilisticFilterTypeCuckoo:
		return f.CreateCuckooFilterFromConfig(name, cfg.Cuckoo, defaults.Cuckoo)
	default:
		return nil, f.Errorf("unknown filter type: %s", cfg.Type)
	}
}

// CreateBloomStorageFromConfig creates a Bloom filter storage from configuration.
func (f *Factory) CreateBloomStorageFromConfig(
	name string,
	cfg *config.ProbabilisticFilterBloomConfig,
	defaults *config.ProbabilisticFilterBloomDefaults,
) (bloomstorages.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if defaults == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storageType := f.bloomStorageType(cfg, defaults)
	expectedItems := cfg.ExpectedItems
	falsePositiveRate := f.bloomFalsePositiveRate(cfg, defaults)

	switch storageType {
	case config.ProbabilisticFilterStorageTypeMemory:
		return bloommemory.New(
			bloommemory.WithExpectedItems(expectedItems),
			bloommemory.WithFalsePositiveRate(falsePositiveRate),
		), nil

	case config.ProbabilisticFilterStorageTypeRedis:
		if err := f.RequireDependency(f.redisClient, "redis client"); err != nil {
			return nil, err
		}
		opts := []bloomredis.Option{
			bloomredis.WithExpectedItems(expectedItems),
			bloomredis.WithFalsePositiveRate(falsePositiveRate),
		}
		opts = slices.AppendIfFunc(opts, cfg.Redis != nil && cfg.Redis.KeysPrefix != "", func() []bloomredis.Option {
			return []bloomredis.Option{bloomredis.WithKeyPrefix(cfg.Redis.KeysPrefix)}
		})
		return bloomredis.New(f.redisClient, name, opts...), nil

	default:
		return nil, f.Errorf("unknown Bloom filter storage type: %s", storageType)
	}
}

// CreateBloomFilterFromConfig creates a Bloom filter from configuration.
func (f *Factory) CreateBloomFilterFromConfig(
	name string,
	cfg *config.ProbabilisticFilterBloomConfig,
	defaults *config.ProbabilisticFilterBloomDefaults,
) (*bloom.Filter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if defaults == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storage, err := f.CreateBloomStorageFromConfig(name, cfg, defaults)
	if err != nil {
		return nil, err
	}
	return bloom.New(storage), nil
}

// CreateCuckooStorageFromConfig creates a Cuckoo filter storage from configuration.
func (f *Factory) CreateCuckooStorageFromConfig(
	name string,
	cfg *config.ProbabilisticFilterCuckooConfig,
	defaults *config.ProbabilisticFilterCuckooDefaults,
) (cuckoostorages.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if defaults == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storageType := f.cuckooStorageType(cfg, defaults)
	capacity := cfg.Capacity

	switch storageType {
	case config.ProbabilisticFilterStorageTypeMemory:
		return cuckoomemory.New(
			//nolint:gosec // G115: capacity is validated positive via config
			cuckoomemory.WithCapacity(uint(capacity)),
		), nil

	case config.ProbabilisticFilterStorageTypeRedis:
		if err := f.RequireDependency(f.redisClient, "redis client"); err != nil {
			return nil, err
		}
		opts := []cuckoeredis.Option{
			cuckoeredis.WithCapacity(capacity),
		}
		opts = slices.AppendIfFunc(opts, cfg.Redis != nil && cfg.Redis.KeysPrefix != "", func() []cuckoeredis.Option {
			return []cuckoeredis.Option{cuckoeredis.WithKeyPrefix(cfg.Redis.KeysPrefix)}
		})
		return cuckoeredis.New(f.redisClient, name, opts...), nil

	default:
		return nil, f.Errorf("unknown Cuckoo filter storage type: %s", storageType)
	}
}

// CreateCuckooFilterFromConfig creates a Cuckoo filter from configuration.
func (f *Factory) CreateCuckooFilterFromConfig(
	name string,
	cfg *config.ProbabilisticFilterCuckooConfig,
	defaults *config.ProbabilisticFilterCuckooDefaults,
) (*cuckoo.Filter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if defaults == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storage, err := f.CreateCuckooStorageFromConfig(name, cfg, defaults)
	if err != nil {
		return nil, err
	}
	return cuckoo.New(storage), nil
}

func (f *Factory) bloomStorageType(
	cfg *config.ProbabilisticFilterBloomConfig,
	defaults *config.ProbabilisticFilterBloomDefaults,
) config.ProbabilisticFilterStorageType {
	storage := cmp.Or(cfg.Storage, &defaults.Storage)
	return cmp.Or(*storage, config.ProbabilisticFilterStorageTypeMemory)
}

func (f *Factory) bloomFalsePositiveRate(cfg *config.ProbabilisticFilterBloomConfig, defaults *config.ProbabilisticFilterBloomDefaults) float64 {
	positiveRate := cmp.Or(cfg.FalsePositiveRate, &defaults.FalsePositiveRate)
	return cmp.Or(*positiveRate, defaultBloomFPR)
}

func (f *Factory) cuckooStorageType(
	cfg *config.ProbabilisticFilterCuckooConfig,
	defaults *config.ProbabilisticFilterCuckooDefaults,
) config.ProbabilisticFilterStorageType {
	storage := cmp.Or(cfg.Storage, &defaults.Storage)
	return cmp.Or(*storage, config.ProbabilisticFilterStorageTypeMemory)
}

// CreateManagerFromConfig creates a Manager with all filters from configuration.
func (f *Factory) CreateManagerFromConfig(cfg *config.ProbabilisticFilter) (*probfilter.Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	mgr := probfilter.NewManager()

	if cfg.Filters == nil {
		return mgr, nil
	}

	for name, filterCfg := range cfg.Filters {
		filter, err := f.CreateFilterFromConfig(name, filterCfg, cfg.Defaults)
		if err != nil {
			// Close any already created filters
			mgr.Close()
			return nil, f.WrapError(err, "failed to create filter "+name)
		}
		if err := mgr.Register(name, filter); err != nil {
			if closer, ok := filter.(io.Closer); ok {
				closer.Close()
			}
			mgr.Close()
			return nil, f.WrapError(err, "failed to register filter "+name)
		}
	}

	return mgr, nil
}

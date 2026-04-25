// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"fmt"
	"io"
	"log/slog"
	"sort"

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

// --- FilterBuilder ---

// FilterBuilder assembles a [probfilter.Filter] step by step using a fluent API.
// Create instances with [NewFilter]. Errors are accumulated and reported at [FilterBuilder.Build] time.
// The builder is not safe for concurrent use.
type FilterBuilder struct {
	corefactory.Base
	name     string
	cfg      *config.ProbabilisticFilterConfig
	defaults *config.ProbabilisticFilterDefaults
	errs     []error

	// Dependencies
	redisClient redis.UniversalClient
}

// NewFilter creates a [FilterBuilder] for the given filter name, config, and defaults.
// Config and defaults can be nil — the errors surface at [FilterBuilder.Build] time.
func NewFilter(name string, cfg *config.ProbabilisticFilterConfig, defaults *config.ProbabilisticFilterDefaults) *FilterBuilder {
	return &FilterBuilder{
		Base:     corefactory.NewBase(slog.New(slog.DiscardHandler)),
		name:     name,
		cfg:      cfg,
		defaults: defaults,
	}
}

// Build assembles the probabilistic filter. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *FilterBuilder) Build() (probfilter.Filter, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if b.defaults == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch b.cfg.Type {
	case config.ProbabilisticFilterTypeBloom:
		return b.createBloomFilter()
	case config.ProbabilisticFilterTypeCuckoo:
		return b.createCuckooFilter()
	default:
		return nil, b.Errorf("unknown filter type: %s", b.cfg.Type)
	}
}

// createBloomFilter creates a Bloom filter from configuration.
func (b *FilterBuilder) createBloomFilter() (*bloom.Filter, error) {
	if b.cfg.Bloom == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if b.defaults.Bloom == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storage, err := b.createBloomStorage()
	if err != nil {
		return nil, err
	}
	return bloom.New(storage), nil
}

// createBloomStorage creates a Bloom filter storage from configuration.
func (b *FilterBuilder) createBloomStorage() (bloomstorages.Storage, error) {
	cfg := b.cfg.Bloom
	storageType := bloomStorageType(cfg, b.defaults.Bloom)
	expectedItems := cfg.ExpectedItems
	falsePositiveRate := bloomFalsePositiveRate(cfg, b.defaults.Bloom)

	switch storageType {
	case config.ProbabilisticFilterStorageTypeMemory:
		return bloommemory.New(
			bloommemory.WithExpectedItems(expectedItems),
			bloommemory.WithFalsePositiveRate(falsePositiveRate),
		), nil

	case config.ProbabilisticFilterStorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		opts := []bloomredis.Option{
			bloomredis.WithExpectedItems(expectedItems),
			bloomredis.WithFalsePositiveRate(falsePositiveRate),
		}
		opts = slices.AppendIfFunc(opts, cfg.Redis != nil && cfg.Redis.KeysPrefix != "", func() []bloomredis.Option {
			return []bloomredis.Option{bloomredis.WithKeyPrefix(cfg.Redis.KeysPrefix)}
		})
		return bloomredis.New(b.redisClient, b.name, opts...), nil

	default:
		return nil, b.Errorf("unknown Bloom filter storage type: %s", storageType)
	}
}

// createCuckooFilter creates a Cuckoo filter from configuration.
func (b *FilterBuilder) createCuckooFilter() (*cuckoo.Filter, error) {
	if b.cfg.Cuckoo == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if b.defaults.Cuckoo == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storage, err := b.createCuckooStorage()
	if err != nil {
		return nil, err
	}
	return cuckoo.New(storage), nil
}

// createCuckooStorage creates a Cuckoo filter storage from configuration.
func (b *FilterBuilder) createCuckooStorage() (cuckoostorages.Storage, error) {
	cfg := b.cfg.Cuckoo
	storageType := cuckooStorageType(cfg, b.defaults.Cuckoo)
	capacity := cfg.Capacity

	switch storageType {
	case config.ProbabilisticFilterStorageTypeMemory:
		return cuckoomemory.New(
			//nolint:gosec // G115: capacity is validated positive via config
			cuckoomemory.WithCapacity(uint(capacity)),
		), nil

	case config.ProbabilisticFilterStorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		opts := []cuckoeredis.Option{
			cuckoeredis.WithCapacity(capacity),
		}
		opts = slices.AppendIfFunc(opts, cfg.Redis != nil && cfg.Redis.KeysPrefix != "", func() []cuckoeredis.Option {
			return []cuckoeredis.Option{cuckoeredis.WithKeyPrefix(cfg.Redis.KeysPrefix)}
		})
		return cuckoeredis.New(b.redisClient, b.name, opts...), nil

	default:
		return nil, b.Errorf("unknown Cuckoo filter storage type: %s", storageType)
	}
}

// --- ManagerBuilder ---

// ManagerBuilder assembles a [probfilter.Manager] step by step using a fluent API.
// Create instances with [NewManager]. Errors are accumulated and reported at [ManagerBuilder.Build] time.
// The builder is not safe for concurrent use.
type ManagerBuilder struct {
	corefactory.Base
	cfg  *config.ProbabilisticFilter
	errs []error

	// Dependencies
	redisClient redis.UniversalClient
}

// NewManager creates a [ManagerBuilder] for the given probabilistic filter config.
// Config can be nil — the error surfaces at [ManagerBuilder.Build] time.
func NewManager(cfg *config.ProbabilisticFilter) *ManagerBuilder {
	return &ManagerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the probabilistic filter manager with all configured filters.
// Errors from fluent methods are accumulated and reported here via [errors.Join].
func (b *ManagerBuilder) Build() (*probfilter.Manager, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	mgr := probfilter.NewManager()

	if b.cfg.Filters == nil {
		return mgr, nil
	}

	names := make([]string, 0, len(b.cfg.Filters))
	for name := range b.cfg.Filters {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		filterCfg := b.cfg.Filters[name]
		filter, err := NewFilter(name, filterCfg, b.cfg.Defaults).
			UseLogger(b.Logger()).
			UseRedisClient(b.redisClient).
			Build()
		if err != nil {
			// Close any already created filters.
			_ = mgr.Close()
			return nil, b.WrapError(err, "failed to create filter "+name)
		}
		if err := mgr.Register(name, filter); err != nil {
			if closer, ok := filter.(io.Closer); ok {
				_ = closer.Close()
			}
			_ = mgr.Close()
			return nil, b.WrapError(err, "failed to register filter "+name)
		}
	}

	return mgr, nil
}

// --- Shared helpers ---

func bloomStorageType(
	cfg *config.ProbabilisticFilterBloomConfig,
	defaults *config.ProbabilisticFilterBloomDefaults,
) config.ProbabilisticFilterStorageType {
	storage := cmp.Or(cfg.Storage, &defaults.Storage)
	return cmp.Or(*storage, config.ProbabilisticFilterStorageTypeMemory)
}

func bloomFalsePositiveRate(cfg *config.ProbabilisticFilterBloomConfig, defaults *config.ProbabilisticFilterBloomDefaults) float64 {
	positiveRate := cmp.Or(cfg.FalsePositiveRate, &defaults.FalsePositiveRate)
	return cmp.Or(*positiveRate, defaultBloomFPR)
}

func cuckooStorageType(
	cfg *config.ProbabilisticFilterCuckooConfig,
	defaults *config.ProbabilisticFilterCuckooDefaults,
) config.ProbabilisticFilterStorageType {
	storage := cmp.Or(cfg.Storage, &defaults.Storage)
	return cmp.Or(*storage, config.ProbabilisticFilterStorageTypeMemory)
}

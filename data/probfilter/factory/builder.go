// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"sort"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
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

	// supportedFingerprintBits is the only Cuckoo fingerprint size the storage
	// backends implement (the in-memory filter and RedisBloom both use 8 bits).
	supportedFingerprintBits = 8

	// rebuildTaskPrefix prefixes the scheduler task ID of a filter rebuild.
	rebuildTaskPrefix = "probfilter-rebuild-"
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
	loader      probfilter.DataLoader
	scheduler   corescheduler.TaskRegistrar
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
//
// For a Bloom filter with a data loader ([FilterBuilder.UseDataLoader]), Build
// honors rebuildOnStart (a synchronous rebuild inside Build) and rebuildCron:
// an in-memory filter is rebuilt by a process-local cron, a Redis filter by a
// task registered with the scheduler ([FilterBuilder.UseScheduler]; without
// one the cron is logged as ignored). A failed initial rebuild, an invalid
// cron or a failed registration closes the filter and fails Build. Without a
// loader the rebuild settings are inert.
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
	filter := bloom.New(storage)
	if err := b.wireBloomRebuild(filter); err != nil {
		_ = filter.Close(context.Background())
		return nil, err
	}
	return filter, nil
}

// cronParser parses rebuildCron like the scheduler does: six fields with
// seconds, plus descriptors such as "@every 5m" and "@hourly".
var cronParser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// wireBloomRebuild applies the rebuildOnStart and rebuildCron settings. They
// need a data source, so without a loader they are inert.
//
// rebuildOnStart always rebuilds synchronously inside Build, so every newly
// created filter is populated regardless of any persisted schedule state.
// rebuildCron is scheduled where the filter's contents live: an in-memory
// filter is local to this process, so it gets a process-local cron (a shared
// scheduler that dispatches on one node would leave the other nodes'
// filters stale); a Redis filter is shared, so it is registered once with the
// injected (possibly distributed) scheduler.
func (b *FilterBuilder) wireBloomRebuild(filter *bloom.Filter) error {
	spec := *cmp.Or(b.cfg.Bloom.RebuildCron, &b.defaults.Bloom.RebuildCron)
	onStart := *cmp.Or(b.cfg.Bloom.RebuildOnStart, &b.defaults.Bloom.RebuildOnStart)

	if nilcheck.IsNil(b.loader) {
		b.Logger().Debug("probfilter: no data loader, rebuild settings are inert", slog.String("filter", b.name))
		return nil
	}

	if onStart {
		if err := filter.Rebuild(context.Background(), b.loader); err != nil {
			return b.WrapError(err, "failed initial rebuild of filter "+b.name)
		}
	}
	if spec == "" {
		return nil
	}

	if bloomStorageType(b.cfg.Bloom, b.defaults.Bloom) == config.ProbabilisticFilterStorageTypeMemory {
		return b.startLocalRebuild(filter, spec)
	}

	if nilcheck.IsNil(b.scheduler) {
		b.Logger().Warn("probfilter: rebuildCron ignored, no scheduler injected for a shared filter",
			slog.String("filter", b.name), slog.String("rebuildCron", spec))
		return nil
	}
	loader := b.loader
	err := b.scheduler.Register(context.Background(), corescheduler.TaskConfig{
		ID:          rebuildTaskPrefix + b.name,
		Description: "Rebuild probabilistic filter " + b.name + " from its data source",
		Schedule:    spec,
		Priority:    corescheduler.TaskPriorityNormal,
		Func: func(ctx context.Context) error {
			// A closed filter has been discarded, and a rebuild in progress on
			// another node publishes a fresh snapshot: nothing to do then.
			err := filter.Rebuild(ctx, loader)
			if err != nil && !errors.Is(err, probfilter.ErrFilterClosed) && !errors.Is(err, probfilter.ErrRebuildInProgress) {
				return err
			}
			return nil
		},
	})
	if err != nil {
		return b.WrapError(err, "failed to register rebuild task for filter "+b.name)
	}
	return nil
}

// startLocalRebuild rebuilds an in-memory filter on a process-local cron that
// is stopped as soon as the filter is closed.
func (b *FilterBuilder) startLocalRebuild(filter *bloom.Filter, spec string) error {
	schedule, err := cronParser.Parse(spec)
	if err != nil {
		return b.WrapError(err, "invalid rebuildCron of filter "+b.name)
	}

	// Skip ticks while a rebuild still runs, so a slow loader cannot pile up
	// queued rebuilds.
	c := cron.New(cron.WithParser(cronParser), cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger)))
	logger, name, loader := b.Logger(), b.name, b.loader
	c.Schedule(schedule, cron.FuncJob(func() {
		err := filter.Rebuild(context.Background(), loader)
		if err != nil && !errors.Is(err, probfilter.ErrFilterClosed) {
			logger.Warn("probfilter: scheduled rebuild failed", slog.String("filter", name), slog.Any("error", err))
		}
	}))
	c.Start()
	go func() {
		<-filter.Done()
		c.Stop()
	}()
	return nil
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
	b.warnUnsupportedCuckooSettings(cfg, storageType)

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
		multiplier := *cmp.Or(cfg.CapacityMultiplier, &b.defaults.Cuckoo.CapacityMultiplier)
		opts = slices.AppendIf(opts, multiplier > 0, cuckoeredis.WithExpansion(int64(math.Ceil(multiplier))))
		opts = slices.AppendIfFunc(opts, cfg.Redis != nil && cfg.Redis.KeysPrefix != "", func() []cuckoeredis.Option {
			return []cuckoeredis.Option{cuckoeredis.WithKeyPrefix(cfg.Redis.KeysPrefix)}
		})
		return cuckoeredis.New(b.redisClient, b.name, opts...), nil

	default:
		return nil, b.Errorf("unknown Cuckoo filter storage type: %s", storageType)
	}
}

// warnUnsupportedCuckooSettings logs per-filter Cuckoo settings the selected
// storage cannot honor. fingerprintSize and maxCapacity are deprecated: no
// backend supports them. capacityMultiplier maps to RedisBloom EXPANSION and
// has no in-memory equivalent.
func (b *FilterBuilder) warnUnsupportedCuckooSettings(
	cfg *config.ProbabilisticFilterCuckooConfig,
	storageType config.ProbabilisticFilterStorageType,
) {
	//nolint:staticcheck // SA1019: read only to warn that the deprecated settings are ignored.
	fingerprintSize, maxCapacity := cfg.FingerprintSize, cfg.MaxCapacity
	if fingerprintSize != nil && *fingerprintSize != supportedFingerprintBits {
		b.Logger().Warn("probfilter: deprecated fingerprintSize ignored, backends use 8-bit fingerprints",
			slog.String("filter", b.name), slog.Int("fingerprintSize", *fingerprintSize))
	}
	if maxCapacity != nil {
		b.Logger().Warn("probfilter: deprecated maxCapacity ignored, no backend enforces it",
			slog.String("filter", b.name), slog.Int64("maxCapacity", *maxCapacity))
	}
	if cfg.CapacityMultiplier != nil && storageType == config.ProbabilisticFilterStorageTypeMemory {
		b.Logger().Warn("probfilter: capacityMultiplier ignored, only the redis storage can grow",
			slog.String("filter", b.name), slog.Float64("capacityMultiplier", *cfg.CapacityMultiplier))
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
	loaders     map[string]probfilter.DataLoader
	scheduler   corescheduler.TaskRegistrar
	collector   metrics.Collector
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

	var mgrOpts []probfilter.Option
	mgrOpts = slices.AppendIf(mgrOpts, b.collector != nil, probfilter.WithCollector(b.collector))
	mgr := probfilter.NewManager(mgrOpts...)

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
			UseDataLoader(b.loaders[name]).
			UseScheduler(b.scheduler).
			Build()
		if err != nil {
			// Close any already created filters; this also turns their
			// registered rebuild tasks into no-ops.
			_ = mgr.Close()
			return nil, b.WrapError(err, "failed to create filter "+name)
		}
		if err := mgr.Register(name, filter); err != nil {
			closeFilter(filter)
			_ = mgr.Close()
			return nil, b.WrapError(err, "failed to register filter "+name)
		}
	}

	return mgr, nil
}

// --- Shared helpers ---

// closeFilter closes filter whether it implements io.Closer or exposes
// Close(context.Context) error like the Bloom and Cuckoo facades.
func closeFilter(filter probfilter.Filter) {
	switch closer := filter.(type) {
	case io.Closer:
		_ = closer.Close()
	case interface{ Close(context.Context) error }:
		_ = closer.Close(context.Background())
	}
}

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

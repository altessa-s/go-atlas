// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"io"
	"log/slog"
	"maps"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/cache/probfilter/bloom"
	"github.com/altessa-s/go-atlas/cache/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/observability/metrics"

	bloomstorages "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages"
	bloommemory "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/memory"
	cuckoostorages "github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages"
	cuckoomemory "github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages/memory"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
	stdslices "slices"
)

const (
	// defaultBloomFPR is the default false positive rate for Bloom filters.
	defaultBloomFPR = 0.01
)

// --- Storage registration ---

// StorageSpec describes the filter whose storage a registered storage
// function builds.
type StorageSpec struct {
	// Name is the filter name; it scopes the storage keys.
	Name string
	// Storage is the per-filter storage block; nil when the filter uses the
	// default storage type.
	Storage *probfilterconfig.Storage
	// Logger is the builder's logger.
	Logger *slog.Logger
	// SkipEvictionPolicyCheck reports that the check of the server's
	// eviction policy is disabled (see [FilterBuilder.SkipEvictionPolicyCheck]).
	SkipEvictionPolicyCheck bool
}

// BloomStorageSpec describes the Bloom filter whose storage a registered
// [BloomStorageFunc] builds.
type BloomStorageSpec struct {
	StorageSpec
	// ExpectedItems is the configured filter size.
	ExpectedItems int64
	// FalsePositiveRate is the false positive rate resolved from the filter
	// and its defaults.
	FalsePositiveRate float64
}

// CuckooStorageSpec describes the Cuckoo filter whose storage a registered
// [CuckooStorageFunc] builds.
type CuckooStorageSpec struct {
	StorageSpec
	// Capacity is the configured filter capacity.
	Capacity int64
	// CapacityMultiplier is the growth factor resolved from the filter and
	// its defaults; zero or less disables growth.
	CapacityMultiplier float64
}

// BloomStorageFunc creates the Bloom filter storage of one storage type. ctx
// bounds the startup I/O of the storage.
type BloomStorageFunc func(ctx context.Context, spec BloomStorageSpec) (bloomstorages.Storage, error)

// CuckooStorageFunc creates the Cuckoo filter storage of one storage type.
// ctx bounds the startup I/O of the storage.
type CuckooStorageFunc func(ctx context.Context, spec CuckooStorageSpec) (cuckoostorages.Storage, error)

// --- FilterBuilder ---

// FilterBuilder assembles a [probfilter.Filter] step by step using a fluent API.
// Create instances with [NewFilter]. Errors are accumulated and reported at [FilterBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds only the in-memory storages. The Redis ones register
// their [BloomStorageFunc] and [CuckooStorageFunc] with
// [FilterBuilder.UseBloomStorage] and [FilterBuilder.UseCuckooStorage] from
// factory/cache/probfilter/bloom/storages/redis and
// factory/cache/probfilter/cuckoo/storages/redis; a configured type without
// one fails at Build naming the package to add.
type FilterBuilder struct {
	corefactory.Base
	name     string
	cfg      *probfilterconfig.Filter
	defaults *probfilterconfig.Defaults
	errs     []error

	// Dependencies
	loader    probfilter.DataLoader
	scheduler schedtask.Registrar

	bloomStorages  map[probfilterconfig.StorageType]BloomStorageFunc
	cuckooStorages map[probfilterconfig.StorageType]CuckooStorageFunc

	// tolerateInProgress lets the initial rebuild lose the rebuild lease of
	// a shared filter; see [FilterBuilder.TolerateRebuildInProgress].
	tolerateInProgress bool
	// skipEvictionCheck disables the Redis eviction-policy check; see
	// [FilterBuilder.SkipEvictionPolicyCheck].
	skipEvictionCheck bool
}

// NewFilter creates a [FilterBuilder] for the given filter name, config, and defaults.
// Config and defaults can be nil — the errors surface at [FilterBuilder.Build] time.
func NewFilter(name string, cfg *probfilterconfig.Filter, defaults *probfilterconfig.Defaults) *FilterBuilder {
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
// maps rebuildOnStart and rebuildCron onto the filter's rebuild options and
// starts its lifecycle ([bloom.Filter.StartRebuild]): rebuildOnStart is a
// synchronous rebuild inside Build, and for rebuildCron
// an in-memory filter is rebuilt by a process-local cron, a Redis filter by a
// task registered with the scheduler ([FilterBuilder.UseScheduler]; without
// one the cron is logged as ignored). A failed initial rebuild, an invalid
// cron or a failed registration closes the filter and fails Build — also an
// initial rebuild of a shared filter refused because another process is
// rebuilding it, unless [FilterBuilder.TolerateRebuildInProgress] was set.
// Without a loader the rebuild settings are inert. ctx bounds the startup I/O
// (the Redis eviction-policy check, the initial rebuild and the rebuild task
// registration); scheduled rebuilds do not inherit it.
func (b *FilterBuilder) Build(ctx context.Context) (probfilter.Filter, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if b.defaults == nil {
		return nil, corefactory.ErrConfigRequired
	}

	switch b.cfg.Type {
	case probfilterconfig.TypeBloom:
		return b.createBloomFilter(ctx)
	case probfilterconfig.TypeCuckoo:
		return b.createCuckooFilter(ctx)
	default:
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unknown filter type: %s", b.cfg.Type)
	}
}

// createBloomFilter creates a Bloom filter from configuration.
func (b *FilterBuilder) createBloomFilter(ctx context.Context) (*bloom.Filter, error) {
	if b.cfg.Bloom == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if b.defaults.Bloom == nil {
		return nil, corefactory.ErrConfigRequired
	}

	storage, err := b.createBloomStorage(ctx)
	if err != nil {
		return nil, err
	}
	filter := bloom.New(storage, b.bloomOptions()...)
	if err := filter.StartRebuild(ctx); err != nil {
		// Cleanup must run even when the failure was ctx being canceled.
		_ = filter.Close(context.WithoutCancel(ctx))
		return nil, b.WrapError(err, "failed to start rebuild of filter "+b.name)
	}
	return filter, nil
}

// bloomOptions maps the rebuildOnStart and rebuildCron settings and the
// injected dependencies onto the rebuild lifecycle of the filter (see
// [bloom.Filter.StartRebuild]).
func (b *FilterBuilder) bloomOptions() []bloom.Option {
	spec := *cmp.Or(b.cfg.Bloom.RebuildCron, &b.defaults.Bloom.RebuildCron)
	onStart := *cmp.Or(b.cfg.Bloom.RebuildOnStart, &b.defaults.Bloom.RebuildOnStart)

	opts := []bloom.Option{
		bloom.WithName(b.name),
		bloom.WithLogger(b.Logger()),
		bloom.WithRebuildLoader(b.loader),
		bloom.WithRebuildSchedule(spec),
		bloom.WithScheduler(b.scheduler),
	}
	opts = slices.AppendIf(opts, onStart, bloom.WithRebuildOnStart())
	return slices.AppendIf(opts, b.tolerateInProgress, bloom.WithTolerateRebuildInProgress())
}

// createBloomStorage creates a Bloom filter storage from configuration.
func (b *FilterBuilder) createBloomStorage(ctx context.Context) (bloomstorages.Storage, error) {
	cfg := b.cfg.Bloom
	storageType := bloomStorageType(cfg, b.defaults.Bloom)
	expectedItems := cfg.ExpectedItems
	falsePositiveRate := bloomFalsePositiveRate(cfg, b.defaults.Bloom)

	if storageType == probfilterconfig.StorageTypeMemory {
		return bloommemory.New(
			bloommemory.WithExpectedItems(expectedItems),
			bloommemory.WithFalsePositiveRate(falsePositiveRate),
		), nil
	}

	if build, ok := b.bloomStorages[storageType]; ok {
		return build(ctx, BloomStorageSpec{
			StorageSpec:       b.storageSpec(cfg.Storage),
			ExpectedItems:     expectedItems,
			FalsePositiveRate: falsePositiveRate,
		})
	}
	if storageType == probfilterconfig.StorageTypeRedis {
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered,
			"probfilter Bloom storage %q is not registered: add factory/cache/probfilter/bloom/storages/redis "+
				"with UseBloomStorage(%q, bloomredisfactory.Builder(client))", storageType, storageType)
	}
	return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unknown Bloom filter storage type: %s", storageType)
}

// createCuckooFilter creates a Cuckoo filter from configuration.
func (b *FilterBuilder) createCuckooFilter(ctx context.Context) (*cuckoo.Filter, error) {
	if b.cfg.Cuckoo == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if b.defaults.Cuckoo == nil {
		return nil, corefactory.ErrConfigRequired
	}

	storage, err := b.createCuckooStorage(ctx)
	if err != nil {
		return nil, err
	}
	return cuckoo.New(storage), nil
}

// createCuckooStorage creates a Cuckoo filter storage from configuration.
func (b *FilterBuilder) createCuckooStorage(ctx context.Context) (cuckoostorages.Storage, error) {
	cfg := b.cfg.Cuckoo
	storageType := cuckooStorageType(cfg, b.defaults.Cuckoo)
	capacity := cfg.Capacity
	b.warnUnsupportedCuckooSettings(cfg, storageType)

	if storageType == probfilterconfig.StorageTypeMemory {
		return cuckoomemory.New(
			//nolint:gosec // G115: capacity is validated positive via config
			cuckoomemory.WithCapacity(uint(capacity)),
		), nil
	}

	if build, ok := b.cuckooStorages[storageType]; ok {
		return build(ctx, CuckooStorageSpec{
			StorageSpec:        b.storageSpec(cfg.Storage),
			Capacity:           capacity,
			CapacityMultiplier: *cmp.Or(cfg.CapacityMultiplier, &b.defaults.Cuckoo.CapacityMultiplier),
		})
	}
	if storageType == probfilterconfig.StorageTypeRedis {
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered,
			"probfilter Cuckoo storage %q is not registered: add factory/cache/probfilter/cuckoo/storages/redis "+
				"with UseCuckooStorage(%q, cuckooredisfactory.Builder(client))", storageType, storageType)
	}
	return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unknown Cuckoo filter storage type: %s", storageType)
}

// storageSpec describes the filter to a registered storage function.
func (b *FilterBuilder) storageSpec(storage *probfilterconfig.Storage) StorageSpec {
	return StorageSpec{
		Name:                    b.name,
		Storage:                 storage,
		Logger:                  b.Logger(),
		SkipEvictionPolicyCheck: b.skipEvictionCheck,
	}
}

// warnUnsupportedCuckooSettings logs per-filter Cuckoo settings the selected
// storage cannot honor. capacityMultiplier maps to RedisBloom EXPANSION and
// has no in-memory equivalent.
func (b *FilterBuilder) warnUnsupportedCuckooSettings(
	cfg *probfilterconfig.CuckooConfig,
	storageType probfilterconfig.StorageType,
) {
	if cfg.CapacityMultiplier != nil && storageType == probfilterconfig.StorageTypeMemory {
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
	cfg  *probfilterconfig.Config
	errs []error

	// Dependencies
	loaders   map[string]probfilter.DataLoader
	scheduler schedtask.Registrar
	collector metrics.Meter

	bloomStorages  map[probfilterconfig.StorageType]BloomStorageFunc
	cuckooStorages map[probfilterconfig.StorageType]CuckooStorageFunc
}

// NewManager creates a [ManagerBuilder] for the given probabilistic filter config.
// Config can be nil — the error surfaces at [ManagerBuilder.Build] time.
func NewManager(cfg *probfilterconfig.Config) *ManagerBuilder {
	return &ManagerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the probabilistic filter manager with all configured filters.
// Errors from fluent methods are accumulated and reported here via [errors.Join].
// ctx bounds the startup I/O of every filter (see [FilterBuilder.Build]).
func (b *ManagerBuilder) Build(ctx context.Context) (*probfilter.Manager, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	var mgrOpts []probfilter.Option
	mgrOpts = slices.AppendIf(mgrOpts, b.collector != nil, probfilter.WithMeter(b.collector))
	mgr := probfilter.NewManager(mgrOpts...)

	if b.cfg.Filters == nil {
		return mgr, nil
	}

	for _, name := range stdslices.Sorted(maps.Keys(b.cfg.Filters)) {
		filterCfg := b.cfg.Filters[name]
		fb := NewFilter(name, filterCfg, b.cfg.Defaults).
			UseLogger(b.Logger()).
			SkipEvictionPolicyCheck(b.cfg.SkipEvictionPolicyCheck).
			UseDataLoader(b.loaders[name]).
			UseScheduler(b.scheduler)
		for typ, build := range b.bloomStorages {
			fb.UseBloomStorage(typ, build)
		}
		for typ, build := range b.cuckooStorages {
			fb.UseCuckooStorage(typ, build)
		}
		filter, err := fb.Build(ctx)
		if err != nil {
			// Close any already created filters; this also turns their
			// registered rebuild tasks into no-ops.
			_ = mgr.Close() //nolint:contextcheck // cleanup is detached from the startup ctx
			return nil, b.WrapError(err, "failed to create filter "+name)
		}
		if err := mgr.Register(name, filter); err != nil {
			closeFilter(filter) //nolint:contextcheck // cleanup is detached from the startup ctx
			_ = mgr.Close()     //nolint:contextcheck // cleanup is detached from the startup ctx
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
	cfg *probfilterconfig.BloomConfig,
	defaults *probfilterconfig.BloomDefaults,
) probfilterconfig.StorageType {
	if cfg.Storage != nil && cfg.Storage.Type != "" {
		return cfg.Storage.Type
	}
	return cmp.Or(defaults.Storage.Type, probfilterconfig.StorageTypeMemory)
}

func bloomFalsePositiveRate(cfg *probfilterconfig.BloomConfig, defaults *probfilterconfig.BloomDefaults) float64 {
	positiveRate := cmp.Or(cfg.FalsePositiveRate, &defaults.FalsePositiveRate)
	return cmp.Or(*positiveRate, defaultBloomFPR)
}

func cuckooStorageType(
	cfg *probfilterconfig.CuckooConfig,
	defaults *probfilterconfig.CuckooDefaults,
) probfilterconfig.StorageType {
	if cfg.Storage != nil && cfg.Storage.Type != "" {
		return cfg.Storage.Type
	}
	return cmp.Or(defaults.Storage.Type, probfilterconfig.StorageTypeMemory)
}

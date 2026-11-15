// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/observability/metrics"

	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
)

// Builder assembles a [negcache.Cache] from a probabilistic-filter config and
// injected dependencies using a fluent API. Missing-dependency and
// filter-construction errors are reported at [Builder.Build] time. The builder
// is not safe for concurrent use.
//
// The negative filter is constructed via the shared probfilter factory and
// wired to the caller-supplied [negcache.Authoritative] revocation store. The
// factory never builds the authoritative store or a Redis storage — the
// caller injects the store and registers the Redis filter storage with
// [Builder.UseBloomStorage] / [Builder.UseCuckooStorage], passed through to
// the probfilter factory.
//
// The probfilter factory owns the filter's rebuilds: the Bloom settings
// rebuildOnStart and rebuildCron rebuild it from the data loader — the one
// set with [Builder.UseDataLoader], else the authoritative store itself when
// it implements [probfilter.DataLoader] (auth/denylist/storages/redis.Store
// does). A Redis filter's rebuildCron runs through the scheduler set with
// [Builder.UseScheduler]. Without a loader the settings are inert and the
// caller rebuilds through [negcache.Cache.Rebuild].
//
// Until a rebuild runs, a revocation made on another node is invisible to
// this node's filter. [Builder.Build] therefore logs a warning whenever the
// built filter has no periodic rebuild scheduled (no loader, an empty
// rebuildCron, a Cuckoo filter, or a shared filter without a scheduler).
type Builder struct {
	corefactory.Base
	name          string
	filterCfg     *probfilterconfig.Filter
	defaults      *probfilterconfig.Defaults
	authoritative negcache.Authoritative
	errs          []error

	// Optional dependencies (set via Use*).
	bloomStorages    map[probfilterconfig.StorageType]probfilterfactory.BloomStorageFunc
	cuckooStorages   map[probfilterconfig.StorageType]probfilterfactory.CuckooStorageFunc
	loader           probfilter.DataLoader
	scheduler        schedtask.Registrar
	metricsCollector metrics.Meter
	metricsSubsystem string
}

// NewBuilder creates a [Builder] for a negative cache named name (the name is
// passed to the probfilter factory for metric and storage-key scoping), built
// from filterCfg and defaults and fronting authoritative.
//
// filterCfg, defaults, and authoritative may be nil here — the errors surface
// at [Builder.Build] time.
func NewBuilder(
	name string,
	filterCfg *probfilterconfig.Filter,
	defaults *probfilterconfig.Defaults,
	authoritative negcache.Authoritative,
) *Builder {
	return &Builder{
		Base:          corefactory.NewBase(slog.New(slog.DiscardHandler)),
		name:          name,
		filterCfg:     filterCfg,
		defaults:      defaults,
		authoritative: authoritative,
	}
}

// Build validates the injected dependencies, builds the negative filter via the
// probfilter factory, and returns a [negcache.Cache] fronting the authoritative
// store. Lookup metrics are wired only when a collector was injected via
// [Builder.UseMetrics]. Accumulated fluent-step errors are reported here.
//
// With a data loader and rebuildOnStart, the filter is rebuilt inside Build
// and the cache is returned populated; a failed rebuild fails Build. When
// another process is rebuilding a shared Redis filter at that moment, Build
// succeeds with an unpopulated cache, which defers to the authoritative
// store until that rebuild is committed. The cache owns the filter: call
// [negcache.Cache.Close] to stop its scheduled rebuilds. ctx bounds the startup
// I/O of the filter build (see probfilter factory FilterBuilder.Build).
func (b *Builder) Build(ctx context.Context) (*negcache.Cache, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	if err := b.RequireDependency(b.authoritative, "authoritative store"); err != nil {
		return nil, err
	}
	if err := b.RequireDependency(b.filterCfg, "filter configuration"); err != nil {
		return nil, err
	}

	loader := b.loader
	if loader == nil {
		loader, _ = b.authoritative.(probfilter.DataLoader)
	}

	// The cache never trusts an unpopulated filter, so a startup rebuild
	// refused because a peer is rebuilding the shared filter is tolerated.
	fb := probfilterfactory.NewFilter(b.name, b.filterCfg, b.defaults).
		UseLogger(b.Logger()).
		UseDataLoader(loader).
		UseScheduler(b.scheduler).
		TolerateRebuildInProgress()
	for typ, build := range b.bloomStorages {
		fb.UseBloomStorage(typ, build)
	}
	for typ, build := range b.cuckooStorages {
		fb.UseCuckooStorage(typ, build)
	}
	filter, err := fb.Build(ctx)
	if err != nil {
		return nil, b.WrapError(err, "failed to build negative filter")
	}

	if reason := b.unscheduledRebuildReason(loader); reason != "" {
		b.Logger().Warn("negcache: no periodic filter rebuild is scheduled; a revocation made on another node "+
			"stays invisible to this node until negcache.Cache.Rebuild runs",
			slog.String("cache", b.name), slog.String("reason", reason))
	}

	var opts []negcache.Option
	opts = slices.AppendIfFunc(opts, b.metricsCollector != nil, func() []negcache.Option {
		return []negcache.Option{negcache.WithMetrics(negcache.NewMetrics(b.metricsCollector, b.metricsSubsystem))}
	})

	return negcache.New(filter, b.authoritative, opts...), nil
}

// unscheduledRebuildReason reports why the probfilter factory schedules no
// periodic rebuild for the filter it just built, or "" when one is scheduled.
// It mirrors the probfilter factory's rules: only a Bloom filter with a data
// loader and a non-empty rebuildCron is rebuilt periodically, and a
// non-memory (shared) filter additionally needs a scheduler.
func (b *Builder) unscheduledRebuildReason(loader probfilter.DataLoader) string {
	if b.filterCfg.Type != probfilterconfig.TypeBloom {
		return "a " + string(b.filterCfg.Type) + " filter has no rebuild schedule"
	}
	if loader == nil {
		return "no data loader: the authoritative store is not a probfilter.DataLoader and UseDataLoader was not called"
	}
	cfg, defaults := b.filterCfg.Bloom, b.defaults.Bloom
	if *cmp.Or(cfg.RebuildCron, &defaults.RebuildCron) == "" {
		return "rebuildCron is empty"
	}
	storageType := cmp.Or(defaults.Storage.Type, probfilterconfig.StorageTypeMemory)
	if cfg.Storage != nil && cfg.Storage.Type != "" {
		storageType = cfg.Storage.Type
	}
	if storageType != probfilterconfig.StorageTypeMemory && b.scheduler == nil {
		return "rebuildCron of a " + string(storageType) + " filter needs UseScheduler"
	}
	return ""
}

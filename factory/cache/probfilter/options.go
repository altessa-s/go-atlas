// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
)

// --- FilterBuilder dependency methods ---

// UseLogger sets the logger for the filter builder and all created components.
func (b *FilterBuilder) UseLogger(v *slog.Logger) *FilterBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *FilterBuilder) UseDefaultLogger() *FilterBuilder {
	return b.UseLogger(slog.Default())
}

// UseBloomStorage registers the Bloom storage function of one storage type
// (probfilterconfig.StorageTypeRedis). A later registration of the same type
// replaces the earlier one; the memory storage is built in.
func (b *FilterBuilder) UseBloomStorage(typ probfilterconfig.StorageType, build BloomStorageFunc) *FilterBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseBloomStorage %q: nil function", typ))
		return b
	}
	if b.bloomStorages == nil {
		b.bloomStorages = map[probfilterconfig.StorageType]BloomStorageFunc{}
	}
	b.bloomStorages[typ] = build
	return b
}

// UseCuckooStorage registers the Cuckoo storage function of one storage type
// (probfilterconfig.StorageTypeRedis). A later registration of the same type
// replaces the earlier one; the memory storage is built in.
func (b *FilterBuilder) UseCuckooStorage(typ probfilterconfig.StorageType, build CuckooStorageFunc) *FilterBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseCuckooStorage %q: nil function", typ))
		return b
	}
	if b.cuckooStorages == nil {
		b.cuckooStorages = map[probfilterconfig.StorageType]CuckooStorageFunc{}
	}
	b.cuckooStorages[typ] = build
	return b
}

// UseDataLoader sets the source a Bloom filter is rebuilt from. Without it the
// rebuildOnStart and rebuildCron settings are inert.
func (b *FilterBuilder) UseDataLoader(v probfilter.DataLoader) *FilterBuilder {
	b.loader = v
	return b
}

// UseScheduler sets the scheduler that runs periodic rebuilds of a Redis
// filter per rebuildCron (an in-memory filter uses a process-local cron). It
// takes effect only together with [FilterBuilder.UseDataLoader].
func (b *FilterBuilder) UseScheduler(v schedtask.Registrar) *FilterBuilder {
	b.scheduler = v
	return b
}

// TolerateRebuildInProgress lets Build succeed when the rebuildOnStart rebuild
// of a shared (Redis) filter is refused because another process is rebuilding
// it ([probfilter.ErrRebuildInProgress]): the filter is returned unpopulated
// (LastRebuild zero) and receives the contents that process publishes. Enable
// it only when the caller never trusts an unpopulated filter — as
// negcache.Cache does, deferring to its authoritative store until
// [probfilter.RebuildCommitReporter] reports a committed rebuild. By default
// such a refusal fails Build, so a filter returned by Build after
// rebuildOnStart is always populated.
func (b *FilterBuilder) TolerateRebuildInProgress() *FilterBuilder {
	b.tolerateInProgress = true
	return b
}

// SkipEvictionPolicyCheck disables, when skip is true, the check that a Redis
// storage's server has no allkeys-* maxmemory-policy; it reaches the
// registered storage function as [StorageSpec.SkipEvictionPolicyCheck]. The
// Redis storages otherwise fail Build with an error wrapping
// [probfilter.ErrUnsafeEvictionPolicy] on such a server, and only log when the
// policy cannot be read (CONFIG denied).
func (b *FilterBuilder) SkipEvictionPolicyCheck(skip bool) *FilterBuilder {
	b.skipEvictionCheck = skip
	return b
}

// --- ManagerBuilder dependency methods ---

// UseLogger sets the logger for the manager builder and all created components.
func (b *ManagerBuilder) UseLogger(v *slog.Logger) *ManagerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ManagerBuilder) UseDefaultLogger() *ManagerBuilder {
	return b.UseLogger(slog.Default())
}

// UseBloomStorage registers the Bloom storage function of one storage type
// for every filter; see [FilterBuilder.UseBloomStorage].
func (b *ManagerBuilder) UseBloomStorage(typ probfilterconfig.StorageType, build BloomStorageFunc) *ManagerBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseBloomStorage %q: nil function", typ))
		return b
	}
	if b.bloomStorages == nil {
		b.bloomStorages = map[probfilterconfig.StorageType]BloomStorageFunc{}
	}
	b.bloomStorages[typ] = build
	return b
}

// UseCuckooStorage registers the Cuckoo storage function of one storage type
// for every filter; see [FilterBuilder.UseCuckooStorage].
func (b *ManagerBuilder) UseCuckooStorage(typ probfilterconfig.StorageType, build CuckooStorageFunc) *ManagerBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseCuckooStorage %q: nil function", typ))
		return b
	}
	if b.cuckooStorages == nil {
		b.cuckooStorages = map[probfilterconfig.StorageType]CuckooStorageFunc{}
	}
	b.cuckooStorages[typ] = build
	return b
}

// UseDataLoader sets the source the filter named name is rebuilt from; see
// [FilterBuilder.UseDataLoader].
func (b *ManagerBuilder) UseDataLoader(name string, v probfilter.DataLoader) *ManagerBuilder {
	if b.loaders == nil {
		b.loaders = make(map[string]probfilter.DataLoader)
	}
	b.loaders[name] = v
	return b
}

// UseScheduler sets the scheduler that runs periodic filter rebuilds; see
// [FilterBuilder.UseScheduler].
func (b *ManagerBuilder) UseScheduler(v schedtask.Registrar) *ManagerBuilder {
	b.scheduler = v
	return b
}

// UseMeter sets the metrics collector of the built [probfilter.Manager],
// enabling the probfilter metrics for every configured filter.
func (b *ManagerBuilder) UseMeter(v metrics.Meter) *ManagerBuilder {
	b.collector = v
	return b
}

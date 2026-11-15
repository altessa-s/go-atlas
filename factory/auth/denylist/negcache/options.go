// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/observability/metrics"

	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
	schedtask "github.com/altessa-s/go-horai/task"
)

// UseLogger sets the logger for the builder and the filter it constructs.
func (b *Builder) UseLogger(v *slog.Logger) *Builder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *Builder) UseDefaultLogger() *Builder {
	return b.UseLogger(slog.Default())
}

// UseBloomStorage registers the Bloom storage function of one storage type,
// passed through to the probfilter factory (FilterBuilder.UseBloomStorage).
// It is required only when a Bloom filter config selects the redis storage
// type: register factory/cache/probfilter/bloom/storages/redis.Builder(client).
// A later registration of the same type replaces the earlier one.
func (b *Builder) UseBloomStorage(typ probfilterconfig.StorageType, build probfilterfactory.BloomStorageFunc) *Builder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseBloomStorage %q: nil function", typ))
		return b
	}
	if b.bloomStorages == nil {
		b.bloomStorages = map[probfilterconfig.StorageType]probfilterfactory.BloomStorageFunc{}
	}
	b.bloomStorages[typ] = build
	return b
}

// UseCuckooStorage registers the Cuckoo storage function of one storage type,
// passed through to the probfilter factory (FilterBuilder.UseCuckooStorage).
// It is required only when a Cuckoo filter config selects the redis storage
// type: register factory/cache/probfilter/cuckoo/storages/redis.Builder(client).
// A later registration of the same type replaces the earlier one.
func (b *Builder) UseCuckooStorage(typ probfilterconfig.StorageType, build probfilterfactory.CuckooStorageFunc) *Builder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseCuckooStorage %q: nil function", typ))
		return b
	}
	if b.cuckooStorages == nil {
		b.cuckooStorages = map[probfilterconfig.StorageType]probfilterfactory.CuckooStorageFunc{}
	}
	b.cuckooStorages[typ] = build
	return b
}

// UseDataLoader sets the source the negative filter is rebuilt from per its
// rebuildOnStart and rebuildCron settings. It overrides the default: the
// authoritative store itself, when it implements [probfilter.DataLoader].
func (b *Builder) UseDataLoader(v probfilter.DataLoader) *Builder {
	b.loader = v
	return b
}

// UseScheduler sets the scheduler that runs the periodic rebuilds of a
// Redis-backed negative filter (rebuildCron); an in-memory filter uses a
// process-local cron instead. See the probfilter factory's UseScheduler.
func (b *Builder) UseScheduler(v schedtask.Registrar) *Builder {
	b.scheduler = v
	return b
}

// UseMetrics enables lookup telemetry on the built cache using the given
// collector and metric subsystem. A nil collector leaves metrics disabled; an
// empty subsystem falls back to [negcache.DefaultMetricsSubsystem].
func (b *Builder) UseMetrics(collector metrics.Meter, subsystem string) *Builder {
	b.metricsCollector = collector
	b.metricsSubsystem = subsystem
	return b
}

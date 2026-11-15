// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	horai "github.com/altessa-s/go-horai"
)

// UseLogger sets the logger for the builder and all created components.
func (b *SchedulerBuilder) UseLogger(v *slog.Logger) *SchedulerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *SchedulerBuilder) UseDefaultLogger() *SchedulerBuilder {
	return b.UseLogger(slog.Default())
}

// UseLeaderElector sets the leader elector for distributed scheduling.
func (b *SchedulerBuilder) UseLeaderElector(v horai.LeaderElector) *SchedulerBuilder {
	b.leaderElector = v
	return b
}

// UseSQLDB sets the database handle for SQL storage backends. The caller owns
// it and chooses the driver matching the configured dialect.
func (b *SchedulerBuilder) UseSQLDB(v *sql.DB) *SchedulerBuilder {
	b.sqlDB = v
	return b
}

// UseStorage registers the task storage function of one storage type
// (schedulerconfig.StorageTypeMongo, schedulerconfig.StorageTypeRedis). A later
// registration of the same type replaces the earlier one; the memory and sqldb
// storages are built in and cannot be replaced.
func (b *SchedulerBuilder) UseStorage(typ schedulerconfig.StorageType, build StorageFunc) *SchedulerBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseStorage %q: nil function", typ))
		return b
	}
	if b.storages == nil {
		b.storages = map[schedulerconfig.StorageType]StorageFunc{}
	}
	b.storages[typ] = build
	return b
}

// UseHistoryStorage registers the history storage builder of one history
// storage type (schedulerconfig.HistoryStorageTypeClickHouse). A later
// registration of the same type replaces the earlier one.
func (b *SchedulerBuilder) UseHistoryStorage(typ schedulerconfig.HistoryStorageType, build HistoryStorageBuilder) *SchedulerBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseHistoryStorage %q: nil builder", typ))
		return b
	}
	if b.histories == nil {
		b.histories = map[schedulerconfig.HistoryStorageType]HistoryStorageBuilder{}
	}
	b.histories[typ] = build
	return b
}

// UseMeter sets the [metrics.Meter] for recording scheduler metrics.
func (b *SchedulerBuilder) UseMeter(v metrics.Meter) *SchedulerBuilder {
	b.collector = v
	return b
}

// UseReadinessProbe sets a readiness probe that defers task dispatch until all
// subsystems signal readiness. See go-horai scheduler.WithReadiness.
func (b *SchedulerBuilder) UseReadinessProbe(fn func() bool) *SchedulerBuilder {
	b.readinessProbe = fn
	return b
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
	schedtask "github.com/altessa-s/go-horai/task"
)

// UseLogger sets the logger for the builder and all created components.
func (b *OutboxBuilder) UseLogger(v *slog.Logger) *OutboxBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *OutboxBuilder) UseDefaultLogger() *OutboxBuilder {
	return b.UseLogger(slog.Default())
}

// UseScheduler sets the task scheduler for background processes.
func (b *OutboxBuilder) UseScheduler(v schedtask.Registrar) *OutboxBuilder {
	b.scheduler = v
	return b
}

// UseOutboxOptions adds code-supplied outbox options — a retry predicate
// ([outbox.WithShouldRetry]), a metrics collector, a compaction filter —
// applied after the options built from configuration, so they win on overlap.
func (b *OutboxBuilder) UseOutboxOptions(opts ...outbox.Option) *OutboxBuilder {
	b.outboxOpts = append(b.outboxOpts, opts...)
	return b
}

// UseStorage registers the storage function of one outbox storage type
// (brokerconfig.OutboxStorageTypeMongo). A later registration of the same type
// replaces the earlier one; the sqldb store is built in.
func (b *OutboxBuilder) UseStorage(typ brokerconfig.OutboxStorageType, build StorageFunc) *OutboxBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseStorage %q: nil function", typ))
		return b
	}
	if b.storages == nil {
		b.storages = map[brokerconfig.OutboxStorageType]StorageFunc{}
	}
	b.storages[typ] = build
	return b
}

// UseSQLDB injects the database handle used when the storage type is "sqldb".
// The caller owns the handle and registers the driver.
func (b *OutboxBuilder) UseSQLDB(v *sql.DB) *OutboxBuilder {
	b.sqlDB = v
	return b
}

// UseSQLListener injects the PostgreSQL notification listener the sqldb store
// watches through when watch is "notify". database/sql has no LISTEN API, so
// the caller adapts its driver; see reliability/outbox/storages/sqldb.
func (b *OutboxBuilder) UseSQLListener(v outboxsql.Listener) *OutboxBuilder {
	b.sqlListener = v
	return b
}

// UseShutdownHooks sets the scope the builder registers the watcher it starts
// into. Without it the watcher is stopped by the process-wide registry — pass
// a group when the outbox must be stoppable on its own.
func (b *OutboxBuilder) UseShutdownHooks(v *coreruntime.HookGroup) *OutboxBuilder {
	b.shutdownHooks = v
	return b
}

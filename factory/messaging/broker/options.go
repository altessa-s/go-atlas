// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	outboxfactory "github.com/altessa-s/go-atlas/factory/reliability/outbox"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
	schedtask "github.com/altessa-s/go-horai/task"
)

// UseLogger sets the logger for the builder and all created components.
func (b *BrokerBuilder) UseLogger(v *slog.Logger) *BrokerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *BrokerBuilder) UseDefaultLogger() *BrokerBuilder {
	return b.UseLogger(slog.Default())
}

// UseScheduler sets the task scheduler for background processes.
func (b *BrokerBuilder) UseScheduler(v schedtask.Registrar) *BrokerBuilder {
	b.scheduler = v
	return b
}

// UseMeter sets the [metrics.Meter] the broker, the provider (through
// [ProviderEnv]) and the in-progress heartbeat manager record their metrics
// on. Without one they record nothing.
func (b *BrokerBuilder) UseMeter(v metrics.Meter) *BrokerBuilder {
	b.collector = v
	return b
}

// UseTracer enables consumer spans on the broker and producer spans on its outbox handlers.
func (b *BrokerBuilder) UseTracer(v tracing.Tracer) *BrokerBuilder {
	b.tracer = v
	return b
}

// UseProvider registers the provider function of one broker provider type
// (brokerconfig.ProviderNATS) for [BrokerBuilder.CreateProvider]. A later
// registration of the same type replaces the earlier one.
func (b *BrokerBuilder) UseProvider(typ brokerconfig.Provider, build ProviderFunc) *BrokerBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseProvider %q: nil function", typ))
		return b
	}
	if b.providers == nil {
		b.providers = map[brokerconfig.Provider]ProviderFunc{}
	}
	b.providers[typ] = build
	return b
}

// UsePublishConverter sets the converter for publish operations.
func (b *BrokerBuilder) UsePublishConverter(v broker.PublishConverter) *BrokerBuilder {
	b.publishConverter = v
	return b
}

// UseOutboxStorage registers the storage function of one outbox storage type
// (brokerconfig.OutboxStorageTypeMongo) for [BrokerBuilder.CreateOutbox]. A
// later registration of the same type replaces the earlier one; the sqldb
// store is built in.
func (b *BrokerBuilder) UseOutboxStorage(typ brokerconfig.OutboxStorageType, build outboxfactory.StorageFunc) *BrokerBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(corefactory.ErrDependencyRequired, "UseOutboxStorage %q: nil function", typ))
		return b
	}
	if b.outboxStorages == nil {
		b.outboxStorages = map[brokerconfig.OutboxStorageType]outboxfactory.StorageFunc{}
	}
	b.outboxStorages[typ] = build
	return b
}

// UseSQLDB injects the database handle the outbox uses when its storage type
// is "sqldb". The caller owns the handle and registers the driver.
func (b *BrokerBuilder) UseSQLDB(v *sql.DB) *BrokerBuilder {
	b.sqlDB = v
	return b
}

// UseSQLListener injects the PostgreSQL notification listener the sqldb outbox
// store watches through when outbox.watch is "notify".
func (b *BrokerBuilder) UseSQLListener(v outboxsql.Listener) *BrokerBuilder {
	b.sqlListener = v
	return b
}

// UseShutdownHooks sets the scope the outbox watcher the builder starts is
// stopped by. Without it the process-wide registry stops it.
func (b *BrokerBuilder) UseShutdownHooks(v *coreruntime.HookGroup) *BrokerBuilder {
	b.shutdownHooks = v
	return b
}

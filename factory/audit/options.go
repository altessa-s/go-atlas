// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"database/sql"
	"log/slog"

	"github.com/altessa-s/go-atlas/audit"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// UseLogger sets the logger for the builder and all created components.
func (b *AuditorBuilder) UseLogger(v *slog.Logger) *AuditorBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *AuditorBuilder) UseDefaultLogger() *AuditorBuilder {
	return b.UseLogger(slog.Default())
}

// UseDispatcher sets the [audit.Dispatcher] that the Auditor will use for
// async event dispatch. The dispatcher must already be started.
//
// Supplying one takes the `storage` and `dispatch` configuration sections out
// of play: the builder wires what it is given rather than building its own
// engine, and does not take responsibility for stopping it.
func (b *AuditorBuilder) UseDispatcher(v audit.Dispatcher) *AuditorBuilder {
	b.dispatcher = v
	return b
}

// UseSQLDB sets the database handle the SQL storage writes to. Required when
// `storage.type` is "sqldb", ignored otherwise. The caller owns it and
// registers the driver.
func (b *AuditorBuilder) UseSQLDB(v *sql.DB) *AuditorBuilder {
	b.sqlDB = v
	return b
}

// UseStorage registers the storage builder of one storage type
// (auditconfig.StorageTypeMongo, auditconfig.StorageTypeClickHouse). A later
// registration of the same type replaces the earlier one; the memory and sqldb
// storages are built in and cannot be replaced.
func (b *AuditorBuilder) UseStorage(typ auditconfig.StorageType, build StorageBuilder) *AuditorBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseStorage %q: nil builder", typ))
		return b
	}
	if b.storages == nil {
		b.storages = map[auditconfig.StorageType]StorageBuilder{}
	}
	b.storages[typ] = build
	return b
}

// UseShutdownHooks sets the scope the builder registers the components it owns
// into. Without it they go into the process-wide registry, which runs once for
// the whole program — pass a group when the audit subsystem must be stoppable
// on its own, e.g. when the caller may tear it down and rebuild it.
func (b *AuditorBuilder) UseShutdownHooks(v *coreruntime.HookGroup) *AuditorBuilder {
	b.shutdownHooks = v
	return b
}

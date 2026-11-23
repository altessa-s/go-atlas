// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	lockconfig "github.com/altessa-s/go-atlas/config/lock"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *DLockBuilder) UseLogger(v *slog.Logger) *DLockBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *DLockBuilder) UseDefaultLogger() *DLockBuilder {
	return b.UseLogger(slog.Default())
}

// UseSQLDB sets the database handle used by the sqldb provider. The caller
// owns the handle and registers the driver.
func (b *DLockBuilder) UseSQLDB(v *sql.DB) *DLockBuilder {
	return b.UseSQLPool(v, nil)
}

// UseSQLPool injects a database and its per-call transaction resolver.
func (b *DLockBuilder) UseSQLPool(db *sql.DB, resolve func(context.Context) (*sql.Tx, bool)) *DLockBuilder {
	b.sqlDB, b.resolveTx = db, resolve
	return b
}

// UseProvider registers the provider function of one provider type
// (lockconfig.DistributionLockProviderNATS,
// lockconfig.DistributionLockProviderMongo). A later registration of the
// same type replaces the earlier one; the sqldb provider is built in.
func (b *DLockBuilder) UseProvider(typ lockconfig.DistributionLockProvider, build ProviderFunc) *DLockBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseProvider %q: nil function", typ))
		return b
	}
	if b.providers == nil {
		b.providers = map[lockconfig.DistributionLockProvider]ProviderFunc{}
	}
	b.providers[typ] = build
	return b
}

// UseHealthCoordinator registers the resulting [dlock.DLock] with the
// supplied health coordinator on construction. Combine with
// [DLockBuilder.UseHealthServiceName] to override the default
// "dlock" service name when several DLock instances share a coordinator.
func (b *DLockBuilder) UseHealthCoordinator(v *health.Coordinator) *DLockBuilder {
	b.healthCoordinator = v
	return b
}

// UseHealthServiceName overrides the service name used for health
// registration. Has no effect unless [DLockBuilder.UseHealthCoordinator]
// is also called.
func (b *DLockBuilder) UseHealthServiceName(v string) *DLockBuilder {
	b.healthServiceName = v
	return b
}

// UseShutdownHooks sets the hook group that closes the built-in SQL provider.
// Without it, Close is registered in the process-wide runtime registry.
func (b *DLockBuilder) UseShutdownHooks(v *coreruntime.HookGroup) *DLockBuilder {
	b.shutdownHooks = v
	return b
}

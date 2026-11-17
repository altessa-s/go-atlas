// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/coord/dlock"
	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/observability/health"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	sqlprovider "github.com/altessa-s/go-atlas/coord/dlock/providers/sqldb"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// ProviderFunc creates the distributed lock provider of one provider type
// from the distribution lock configuration. ctx bounds provider
// provisioning, such as creating a bucket or reconciling indexes; logger is
// the builder's.
type ProviderFunc func(ctx context.Context, cfg *lockconfig.DistributionLock, logger *slog.Logger) (providers.Provider, error)

// DLockBuilder assembles a [dlock.DLock] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [DLockBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds only the sqldb provider. The NATS and MongoDB ones
// register their [ProviderFunc] with [DLockBuilder.UseProvider] from
// factory/coord/dlock/providers/nats and
// factory/coord/dlock/providers/mongo; a configured provider without one
// fails at Build naming the package to add.
type DLockBuilder struct {
	corefactory.Base
	cfg  *lockconfig.DistributionLock
	errs []error

	// Dependencies
	shutdownHooks     *coreruntime.HookGroup
	sqlDB             *sql.DB
	healthCoordinator *health.Coordinator
	healthServiceName string

	providers map[lockconfig.DistributionLockProvider]ProviderFunc
}

// New creates a [DLockBuilder] for the given distribution lock config.
// Config can be nil — the error surfaces at [DLockBuilder.Build] time.
func New(cfg *lockconfig.DistributionLock) *DLockBuilder {
	return &DLockBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the distributed lock. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *DLockBuilder) Build(ctx context.Context) (*dlock.DLock, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	if b.cfg.Provider == lockconfig.DistributionLockProviderSQL {
		return b.createSQLDLock(ctx)
	}
	if build, ok := b.providers[b.cfg.Provider]; ok {
		prov, err := build(ctx, b.cfg, b.Logger())
		if err != nil {
			return nil, err
		}
		return dlock.New(prov, b.applyDefaults()...), nil
	}
	switch typ := b.cfg.Provider; typ {
	case lockconfig.DistributionLockProviderNATS:
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "dlock provider %q is not registered: add factory/coord/dlock/providers/nats "+
			"with UseProvider(%q, natsfactory.Builder(conn))", typ, typ)
	case lockconfig.DistributionLockProviderMongo:
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "dlock provider %q is not registered: add factory/coord/dlock/providers/mongo "+
			"with UseProvider(%q, mongofactory.Builder(db))", typ, typ)
	default:
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unknown distribution lock provider: %s", typ)
	}
}

// createSQLDLock creates a DLock with the SQL provider. With EnsureSchema set
// it creates the table through the provider's idempotent EnsureSchema, bounded
// by ensureSchemaTimeout; otherwise it performs no I/O.
func (b *DLockBuilder) createSQLDLock(ctx context.Context) (*dlock.DLock, error) {
	if b.cfg.SQL == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if err := b.RequireDependency(b.sqlDB, "sql database"); err != nil {
		return nil, err
	}
	cfg := b.cfg.SQL
	provOpts := []sqlprovider.Option{sqlprovider.WithLogger(b.Logger()), sqlprovider.WithTableName(cfg.Table),
		sqlprovider.WithPurgeInterval(cfg.PurgeInterval), sqlprovider.WithPurgeBatchSize(cfg.PurgeBatchSize)}
	provOpts = coreslices.AppendIf(provOpts, cfg.TTL > 0, sqlprovider.WithTTL(cfg.TTL))
	provOpts = coreslices.AppendIf(provOpts, cfg.RenewRatio > 0, sqlprovider.WithRenewRatio(cfg.RenewRatio))
	provOpts = coreslices.AppendIf(provOpts, cfg.WithoutRenewal, sqlprovider.WithoutRenewal())
	provOpts = coreslices.AppendIf(provOpts, cfg.ExpiredCleanup, sqlprovider.WithExpiredCleanup(cfg.ExpiredCleanupGrace))
	prov, err := sqlprovider.New(b.sqlDB, sqlprovider.Dialect(cfg.Dialect), provOpts...)
	if err != nil {
		return nil, coreerrs.Provider("sql distributed lock", err)
	}
	if cfg.EnsureSchema {
		schemaCtx, cancel := context.WithTimeout(ctx, ensureSchemaTimeout)
		defer cancel()
		if err := prov.EnsureSchema(schemaCtx); err != nil {
			return nil, coreerrs.Provider("sql distributed lock", err)
		}
	}
	if b.shutdownHooks != nil {
		b.shutdownHooks.OnShutdown(prov.Close)
	} else {
		coreruntime.OnShutdown(prov.Close)
	}
	return dlock.New(prov, b.applyDefaults()...), nil
}

// ensureSchemaTimeout bounds the schema setup createSQLDLock runs when the
// config asks for it, so a lock wait cannot stall startup indefinitely.
const ensureSchemaTimeout = 30 * time.Second

// applyDefaults returns builder default options.
func (b *DLockBuilder) applyDefaults() []dlock.Option {
	opts := []dlock.Option{dlock.WithLogger(b.Logger())}
	opts = coreslices.AppendIf(opts, b.healthCoordinator != nil,
		dlock.WithHealthCoordinator(b.healthCoordinator))
	opts = coreslices.AppendIf(opts, b.healthServiceName != "",
		dlock.WithHealthServiceName(b.healthServiceName))
	return opts
}

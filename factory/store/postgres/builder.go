// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"maps"
	"slices"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/store/postgres"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	postgresconfig "github.com/altessa-s/go-atlas/config/postgres"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// Builder constructs application pools and registers their health checks after successful startup.
type Builder struct {
	corefactory.Base
	cfg    *postgresconfig.Config
	meter  metrics.Meter
	tracer tracing.Tracer
	health health.Registrar
	source fs.FS
	schema string
}

// New creates a builder. Configuration errors surface at Build.
func New(cfg *postgresconfig.Config) *Builder {
	return &Builder{Base: corefactory.NewBase(nil), cfg: cfg}
}

// Build opens all pools, optionally migrates the primary database and registers health checks.
// The caller owns the returned pools. A failed build closes every pool that it opened.
func (b *Builder) Build(ctx context.Context) (postgres.Pools, error) {
	c := b.cfg
	if c == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.Migrations && (nilcheck.IsNil(b.source) || b.schema == "") {
		return nil, corefactory.Errorf(corefactory.ErrDependencyRequired, "enabled migrations require a source and ledger schema")
	}
	pools := make(postgres.Pools, len(c.Pools)+1)
	for _, name := range append([]string{postgres.DefaultPool}, slices.Sorted(maps.Keys(c.Pools))...) {
		p := c.Pools[name]
		size, idle := c.MaxOpenConnections, c.MaxIdleConnections
		if name != postgres.DefaultPool {
			size = p.MaxOpenConnections
			idle = min(idle, size)
			if p.MaxIdleConnections != nil {
				idle = *p.MaxIdleConnections
			}
		}
		db, err := postgres.Open(ctx, cmp.Or(p.DSN, c.DSN),
			postgres.WithPoolName(name),
			postgres.WithPooling(postgres.PoolingMode(cmp.Or(p.Pooling, c.Pooling))),
			postgres.WithMaxOpenConnections(size),
			postgres.WithMaxIdleConnections(idle),
			postgres.WithConnectionMaxLifetime(c.ConnectionMaxLifetime),
			postgres.WithConnectionMaxIdleTime(c.ConnectionMaxIdleTime),
			postgres.WithConnectTimeout(c.ConnectTimeout),
			postgres.WithStatementTimeout(cmp.Or(p.StatementTimeout, c.StatementTimeout)),
			postgres.WithLockTimeout(cmp.Or(p.LockTimeout, c.LockTimeout)),
			postgres.WithIdleTransactionTimeout(c.IdleTransactionTimeout),
			postgres.WithTracer(b.tracer),
			postgres.WithMeter(b.meter),
			postgres.WithLogger(b.Logger()),
		)
		if err != nil {
			return nil, errors.Join(coreerrs.WrapOperationWithContext(err, "open PostgreSQL pool", name), pools.Close())
		}
		pools[name] = db
	}
	if c.Migrations {
		migrationCtx, cancel := context.WithTimeout(ctx, c.MigrationTimeout)
		err := postgres.Migrate(migrationCtx, pools[postgres.DefaultPool], b.source, b.schema)
		cancel()
		if err != nil {
			return nil, errors.Join(err, pools.Close())
		}
	}
	if !nilcheck.IsNil(b.health) {
		for name, db := range pools {
			b.health.RegisterService("postgres."+name, postgres.Checker(db))
		}
	}
	return pools, nil
}

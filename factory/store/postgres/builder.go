// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"maps"
	"slices"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/store/postgres"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	postgresconfig "github.com/altessa-s/go-atlas/config/postgres"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// Builder constructs application pools and registers their health checks after successful startup.
type Builder struct {
	corefactory.Base
	cfg      *postgresconfig.Config
	meter    metrics.Meter
	tracer   tracing.Tracer
	health   health.Registrar
	migrator func(context.Context, *sql.DB) error
}

// New creates a builder. Configuration errors surface at Build.
func New(cfg *postgresconfig.Config) *Builder {
	return &Builder{Base: corefactory.NewBase(nil), cfg: cfg}
}

// Build opens all pools, optionally migrates the primary database and registers health checks.
// The caller owns the returned pools. A failed build closes every pool that it opened.
func (b *Builder) Build(ctx context.Context) (_ postgres.Pools, buildErr error) {
	c := b.cfg
	if c == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if b.migrator != nil && c.Pooling == string(postgres.PoolingTransaction) {
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "startup migrations require a direct primary pool")
	}
	pools := make(postgres.Pools, len(c.Pools)+1)
	complete := false
	defer func() {
		if !complete {
			buildErr = errors.Join(buildErr, pools.Close())
		}
	}()
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
			return nil, coreerrs.WrapOperationWithContext(err, "open PostgreSQL pool", name)
		}
		pools[name] = db
	}
	if b.migrator != nil {
		migrationCtx, cancel := corecontext.WithMaxTimeout(ctx, c.MigrationTimeout)
		defer cancel()
		err := b.migrator(migrationCtx, pools[postgres.DefaultPool])
		if err != nil {
			return nil, err
		}
	}
	if !nilcheck.IsNil(b.health) {
		for name, db := range pools {
			b.health.RegisterService("postgres."+name, postgres.Checker(db))
		}
	}
	complete = true
	return pools, nil
}

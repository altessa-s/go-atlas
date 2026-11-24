// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"maps"
	"slices"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/store/postgres"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	postgresconfig "github.com/altessa-s/go-atlas/config/postgres"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	tlsfactory "github.com/altessa-s/go-atlas/factory/ext/tlsutils"
)

// Builder constructs application pools and registers their health checks after successful startup.
type Builder struct {
	corefactory.Base
	cfg       *postgresconfig.Config
	tlsConfig *tls.Config
	meter     metrics.Meter
	tracer    tracing.Tracer
	health    health.Registrar
	migrator  func(context.Context, *sql.DB) error
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
	if err := b.validate(); err != nil {
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "invalid PostgreSQL configuration: %w", err)
	}
	if b.migrator != nil && c.Pooling == string(postgres.PoolingTransaction) {
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "startup migrations require a direct primary pool")
	}
	configs, err := b.poolTLSConfigs()
	if err != nil {
		return nil, err
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
		tlsCfg := cmp.Or(configs[name], configs[postgres.DefaultPool])
		opts := []postgres.Option{
			postgres.WithTLSConfig(tlsCfg),
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
		}
		opts = coreslices.AppendIf(opts, tlsCfg != nil && tlsCfg.InsecureSkipVerify, postgres.WithAllowInsecureSkipVerify())
		db, err := postgres.Open(ctx, cmp.Or(p.DSN, c.DSN), opts...)
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

// Validate effective settings. An injected config supersedes certificate files,
// but an explicit TLS block must still agree with its pool's DSN.
func (b *Builder) validate() error {
	if b.tlsConfig == nil {
		return b.cfg.Validate()
	}
	if b.tlsConfig.InsecureSkipVerify {
		return errors.New("injected PostgreSQL TLS configuration must verify the server")
	}
	c := *b.cfg
	if c.TLS != nil {
		c.TLS = &tlsconfig.Client{}
	}
	c.Pools = maps.Clone(c.Pools)
	for name, pool := range c.Pools {
		if pool.TLS != nil {
			pool.TLS = &tlsconfig.Client{}
			c.Pools[name] = pool
		}
	}
	return c.Validate()
}

// Resolve all certificate material before opening even the primary pool.
func (b *Builder) poolTLSConfigs() (map[string]*tls.Config, error) {
	configs := map[string]*tls.Config{postgres.DefaultPool: b.tlsConfig}
	if b.tlsConfig != nil {
		return configs, nil
	}
	builder := tlsfactory.New(nil).UseLogger(b.Logger())
	build := func(name string, block *tlsconfig.Client) error {
		cfg, err := builder.CreateClientConfig(block)
		if err != nil {
			// File errors can contain arbitrary user-supplied paths.
			return corefactory.Errorf(corefactory.ErrUnsupported, "cannot build PostgreSQL TLS configuration for pool %q", name)
		}
		configs[name] = cfg
		return nil
	}
	if err := build(postgres.DefaultPool, b.cfg.TLS); err != nil {
		return nil, err
	}
	for name, pool := range b.cfg.Pools {
		if pool.TLS != nil {
			if err := build(name, pool.TLS); err != nil {
				return nil, err
			}
		}
	}
	return configs, nil
}

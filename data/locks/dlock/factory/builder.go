// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/locks/dlock"
	"github.com/altessa-s/go-atlas/observability/health"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// DLockBuilder assembles a [dlock.DLock] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [DLockBuilder.Build] time.
// The builder is not safe for concurrent use.
type DLockBuilder struct {
	corefactory.Base
	cfg  *config.DistributionLock
	errs []error

	// Dependencies
	natsConn          *nats.Conn
	healthCoordinator *health.Coordinator
	healthServiceName string
}

// New creates a [DLockBuilder] for the given distribution lock config.
// Config can be nil — the error surfaces at [DLockBuilder.Build] time.
func New(cfg *config.DistributionLock) *DLockBuilder {
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
		return nil, fmt.Errorf("configuration is required")
	}

	switch b.cfg.Provider {
	case config.DistributionLockProviderNats:
		return b.createNatsDLock(ctx)
	default:
		return nil, b.Errorf("unknown distribution lock provider: %s", b.cfg.Provider)
	}
}

// createNatsDLock creates a DLock with NATS provider.
func (b *DLockBuilder) createNatsDLock(ctx context.Context) (*dlock.DLock, error) {
	if b.cfg.Nats == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if err := b.RequireDependency(b.natsConn, "nats connection"); err != nil {
		return nil, err
	}

	return dlock.NewWithNats(ctx, b.natsConn, b.cfg.Nats.Bucket, b.applyDefaults()...)
}

// applyDefaults returns builder default options.
func (b *DLockBuilder) applyDefaults() []dlock.Option {
	opts := []dlock.Option{dlock.WithLogger(b.Logger())}
	opts = coreslices.AppendIf(opts, b.healthCoordinator != nil,
		dlock.WithHealthCoordinator(b.healthCoordinator))
	opts = coreslices.AppendIf(opts, b.healthServiceName != "",
		dlock.WithHealthServiceName(b.healthServiceName))
	return opts
}

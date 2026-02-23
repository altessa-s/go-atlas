// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/locks/dlock"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// Factory creates distributed locks from configuration.
type Factory struct {
	corefactory.Base
	natsConn *nats.Conn
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:     corefactory.NewBase(cfg.logger),
		natsConn: cfg.natsConn,
	}
}

// CreateDLockFromConfig creates a DLock from configuration.
func (f *Factory) CreateDLockFromConfig(ctx context.Context, cfg *config.DistributionLock) (*dlock.DLock, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch cfg.Provider {
	case config.DistributionLockProviderNats:
		return f.CreateNatsDLockFromConfig(ctx, cfg.Nats)
	default:
		return nil, f.Errorf("unknown distribution lock provider: %s", cfg.Provider)
	}
}

// CreateNatsDLockFromConfig creates a DLock with NATS provider.
func (f *Factory) CreateNatsDLockFromConfig(ctx context.Context, cfg *config.DistributionLockNats) (*dlock.DLock, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if err := f.RequireDependency(f.natsConn, "nats connection"); err != nil {
		return nil, err
	}

	return dlock.NewWithNats(ctx, f.natsConn, cfg.Bucket, f.applyDefaults()...)
}

// CreateNoopDLock creates a DLock with no-op provider for testing.
func (f *Factory) CreateNoopDLock() *dlock.DLock {
	return dlock.NewWithNoop(f.applyDefaults()...)
}

// applyDefaults returns factory default options.
func (f *Factory) applyDefaults() []dlock.Option {
	return []dlock.Option{dlock.WithLogger(f.Logger())}
}

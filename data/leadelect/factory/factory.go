// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/leadelect"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	natsprovider "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

// Factory creates leader electors from configuration.
type Factory struct {
	corefactory.Base
	natsConn *nats.Conn
	key      string
	nodeId   string
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:     corefactory.NewBase(cfg.logger),
		natsConn: cfg.natsConn,
		key:      cfg.key,
		nodeId:   cfg.nodeId,
	}
}

// CreateLeaderFromConfig creates a Leader from configuration.
func (f *Factory) CreateLeaderFromConfig(ctx context.Context, cfg *config.LeaderElector) (*leadelect.Leader, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	provider, err := f.createNatsProvider(ctx)
	if err != nil {
		return nil, err
	}

	leCfg := f.buildConfig(cfg)
	return leadelect.New(provider, leCfg), nil
}

// createNatsProvider creates a NATS leader election provider.
func (f *Factory) createNatsProvider(ctx context.Context) (*natsprovider.Provider, error) {
	if err := f.RequireDependency(f.natsConn, "nats connection"); err != nil {
		return nil, err
	}

	opts := []natsprovider.Option{
		natsprovider.WithLogger(f.Logger()),
	}

	provider, err := natsprovider.New(ctx, f.natsConn, opts...)
	if err != nil {
		return nil, f.WrapError(err, "failed to create nats provider")
	}

	return provider, nil
}

// buildConfig creates leadelect.Config from factory state and config.
func (f *Factory) buildConfig(cfg *config.LeaderElector) leadelect.Config {
	return leadelect.Config{
		Key:    f.key,
		TTL:    cfg.Ttl,
		NodeId: f.nodeId,
	}
}

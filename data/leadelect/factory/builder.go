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
	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	natsprovider "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

// LeaderBuilder assembles a [leadelect.Leader] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [LeaderBuilder.Build] time.
// The builder is not safe for concurrent use.
type LeaderBuilder struct {
	corefactory.Base
	cfg  *config.LeaderElector
	errs []error

	// Dependencies
	natsConn  *nats.Conn
	collector metrics.Collector

	// Config
	key    string
	nodeId string
}

// New creates a [LeaderBuilder] for the given leader elector config.
// Config can be nil — the error surfaces at [LeaderBuilder.Build] time.
// By default, key is set to [appinfo.Name].
func New(cfg *config.LeaderElector) *LeaderBuilder {
	return &LeaderBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
		key:  appinfo.Name,
	}
}

// Build assembles the leader elector. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *LeaderBuilder) Build(ctx context.Context) (*leadelect.Leader, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	provider, err := b.createNatsProvider(ctx)
	if err != nil {
		return nil, err
	}

	leCfg := b.buildConfig()
	return leadelect.New(provider, leCfg, leadelect.WithCollector(b.collector)), nil
}

// createNatsProvider creates a NATS leader election provider.
func (b *LeaderBuilder) createNatsProvider(ctx context.Context) (*natsprovider.Provider, error) {
	if err := b.RequireDependency(b.natsConn, "nats connection"); err != nil {
		return nil, err
	}

	opts := []natsprovider.Option{
		natsprovider.WithLogger(b.Logger()),
		natsprovider.WithCollector(b.collector),
	}

	provider, err := natsprovider.New(ctx, b.natsConn, opts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create nats provider")
	}

	return provider, nil
}

// buildConfig creates leadelect.Config from builder state and config.
func (b *LeaderBuilder) buildConfig() leadelect.Config {
	return leadelect.Config{
		Key:    b.key,
		TTL:    b.cfg.Ttl,
		NodeId: b.nodeId,
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/observability/metrics"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	natsprovider "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

// LeaderBuilder assembles a [leadelect.Leader] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [LeaderBuilder.Build] time.
// The builder is not safe for concurrent use.
type LeaderBuilder struct {
	corefactory.Base
	cfg  *lockconfig.LeaderElector
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
func New(cfg *lockconfig.LeaderElector) *LeaderBuilder {
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

	return leadelect.New(provider, b.key, b.nodeId,
		leadelect.WithTTL(b.cfg.Ttl),
		leadelect.WithCollector(b.collector),
	), nil
}

// createNatsProvider creates a NATS leader election provider.
func (b *LeaderBuilder) createNatsProvider(ctx context.Context) (*natsprovider.Provider, error) {
	if err := b.RequireDependency(b.natsConn, "nats connection"); err != nil {
		return nil, err
	}

	opts := []natsprovider.Option{
		natsprovider.WithLogger(b.Logger()),
		natsprovider.WithCollector(b.collector),
		natsprovider.WithStorage(bucketStorage(b.cfg.Storage)),
	}
	opts = coreslices.AppendIf(opts, b.cfg.MigrateBucketTTL, natsprovider.WithMigrateBucketTTL())
	opts = coreslices.AppendIf(opts, b.cfg.StrictBucketStorage, natsprovider.WithStrictBucketStorage())

	provider, err := natsprovider.New(ctx, b.natsConn, opts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create nats provider")
	}

	return provider, nil
}

// bucketStorage maps the configured bucket storage onto JetStream's: file is
// file storage, anything else — including unset — the memory default.
func bucketStorage(v storageconfig.KVStorageType) jetstream.StorageType {
	if v == storageconfig.KVStorageFile {
		return jetstream.FileStorage
	}
	return jetstream.MemoryStorage
}

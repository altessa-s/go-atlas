// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	natsstorage "github.com/altessa-s/go-atlas/coord/limiters/storages/nats"
	storagesfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

var (
	// ErrClientRequired is returned when the NATS limiter storage is built
	// without a JetStream context.
	ErrClientRequired = errors.New("limiters: jetstream is required for storage type nats")
	// ErrConfigRequired is returned when the storage configuration has no
	// nats block.
	ErrConfigRequired = fmt.Errorf("limiters: nats storage configuration is required: %w", corefactory.ErrConfigRequired)
)

// Builder returns the storage function of the nats type, ready for UseStorage
// of the budget and token-bucket limiter factories. The storage keeps
// counters in a JetStream KeyValue bucket created through js on the Build
// context; the limiter's natsContention settings tune its compare-and-swap
// retries.
func Builder(js jetstream.JetStream) storagesfactory.StorageFunc {
	return func(ctx context.Context, cfg *storageconfig.BackendConfig, contention *limiterconfig.NATSContention) (storages.Storage, error) {
		if js == nil {
			return nil, ErrClientRequired
		}
		if cfg.NATS == nil {
			return nil, ErrConfigRequired
		}

		opts := []natsstorage.Option{
			natsstorage.WithBucket(cfg.NATS.Bucket),
			natsstorage.WithMaxAge(storagesfactory.NATSRetention),
			natsstorage.WithReplicas(cfg.NATS.Replicas),
		}
		opts = coreslices.AppendIf(opts, cfg.NATS.MigrateBucketTTL, natsstorage.WithMigrateBucketTTL())
		if c := contention; c != nil {
			opts = coreslices.AppendIf(opts, c.MaxCASAttempts > 0, natsstorage.WithMaxCASAttempts(c.MaxCASAttempts))
			opts = coreslices.AppendIf(opts, c.MaxCASBackoff > 0, natsstorage.WithMaxCASBackoff(c.MaxCASBackoff))
		}

		return natsstorage.New(ctx, js, opts...)
	}
}

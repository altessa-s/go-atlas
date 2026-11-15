// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/reliability/saga"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	sagafactory "github.com/altessa-s/go-atlas/factory/reliability/saga"
	natsstore "github.com/altessa-s/go-atlas/reliability/saga/storages/nats"
)

var (
	// ErrClientRequired is returned when the NATS saga store is built without
	// a JetStream context.
	ErrClientRequired = errors.New("saga: NATS JetStream is required for storage type nats")
	// ErrConfigRequired is returned when the storage configuration has no
	// nats block.
	ErrConfigRequired = errors.New("saga: nats configuration is required for storage type nats")
)

// Builder returns the storage function of the nats type, ready for
// sagafactory.Builder.UseStorage. The store keeps instances in the configured
// JetStream KV bucket; ctx bounds the bucket creation.
func Builder(js jetstream.JetStream) sagafactory.StorageFunc {
	return func(ctx context.Context, cfg *sagaconfig.StorageConfig) (saga.Storage, error) {
		if js == nil {
			return nil, ErrClientRequired
		}
		if cfg.NATS == nil {
			return nil, ErrConfigRequired
		}
		opts := []natsstore.Option{
			natsstore.WithBucket(cfg.NATS.Bucket),
			natsstore.WithBucketTTL(cfg.NATS.MaxAge),
		}
		opts = coreslices.AppendIf(opts, cfg.NATS.MigrateBucketTTL, natsstore.WithMigrateBucketTTL())
		opts = coreslices.AppendIf(opts, cfg.NATS.StrictBucketStorage, natsstore.WithStrictBucketStorage())
		return natsstore.New(ctx, js, opts...)
	}
}

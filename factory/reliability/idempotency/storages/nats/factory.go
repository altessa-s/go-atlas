// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	idempotencyfactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
	natsstorage "github.com/altessa-s/go-atlas/reliability/idempotency/storages/nats"
)

var (
	// ErrClientRequired is returned when the NATS idempotency storage is
	// built without a JetStream context.
	ErrClientRequired = errors.New("idempotency: jetstream is required for storage type nats")
	// ErrConfigRequired is returned when the storage configuration has no
	// nats block.
	ErrConfigRequired = fmt.Errorf("idempotency: nats configuration is required for storage type nats: %w", corefactory.ErrConfigRequired)
)

// Builder returns the storage function of the nats type, ready for
// idempotencyfactory.KeeperBuilder.UseStorage. The storage keeps keys in the
// configured JetStream KV bucket, expiring them after the configured TTL.
func Builder(js jetstream.JetStream) idempotencyfactory.StorageFunc {
	return func(cfg *idempotencyconfig.Config) (storages.Storage, error) {
		if js == nil {
			return nil, ErrClientRequired
		}
		if cfg.Storage == nil || cfg.Storage.NATS == nil {
			return nil, ErrConfigRequired
		}
		nc := cfg.Storage.NATS
		opts := []natsstorage.Option{
			natsstorage.WithMaxAge(cfg.TTL),
			natsstorage.WithBucket(nc.Bucket),
			natsstorage.WithReplicas(nc.Replicas),
		}
		opts = coreslices.AppendIf(opts, nc.MigrateBucketTTL, natsstorage.WithMigrateBucketTTL())
		return natsstorage.New(js, opts...)
	}
}

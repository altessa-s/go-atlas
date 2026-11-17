// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	natsprovider "github.com/altessa-s/go-atlas/coord/leadelect/providers/nats"
	leadelectfactory "github.com/altessa-s/go-atlas/factory/coord/leadelect"
)

// ErrClientRequired is returned when the NATS leader election provider is
// built without a connection.
var ErrClientRequired = errors.New("leadelect: nats connection is required for provider nats")

// Builder returns the provider function of the nats type, ready for
// leadelectfactory.LeaderBuilder.UseProvider. The provider keeps the election
// in a JetStream KeyValue bucket created through conn on the Build context.
func Builder(conn *nats.Conn) leadelectfactory.ProviderFunc {
	return func(ctx context.Context, cfg *lockconfig.LeaderElector, logger *slog.Logger, meter metrics.Meter) (providers.Provider, error) {
		if conn == nil {
			return nil, ErrClientRequired
		}

		opts := []natsprovider.Option{
			natsprovider.WithLogger(logger),
			natsprovider.WithMeter(meter),
			natsprovider.WithStorage(bucketStorage(cfg.Storage)),
		}
		opts = coreslices.AppendIf(opts, cfg.MigrateBucketTTL, natsprovider.WithMigrateBucketTTL())

		provider, err := natsprovider.New(ctx, conn, opts...)
		if err != nil {
			return nil, err
		}
		return provider, nil
	}
}

// bucketStorage maps the configured bucket storage onto JetStream's: file is
// file storage, anything else — including unset — the memory default.
func bucketStorage(v storageconfig.KVStorageType) jetstream.StorageType {
	if v == storageconfig.KVStorageFile {
		return jetstream.FileStorage
	}
	return jetstream.MemoryStorage
}

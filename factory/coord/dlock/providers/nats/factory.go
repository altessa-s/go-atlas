// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/coord/dlock/providers"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	natsprovider "github.com/altessa-s/go-atlas/coord/dlock/providers/nats"
	dlockfactory "github.com/altessa-s/go-atlas/factory/coord/dlock"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

var (
	// ErrClientRequired is returned when the NATS lock provider is built
	// without a connection.
	ErrClientRequired = errors.New("dlock: nats connection is required for provider nats")
	// ErrConfigRequired is returned when the distribution lock configuration
	// has no nats block.
	ErrConfigRequired = fmt.Errorf("dlock: nats configuration is required for provider nats: %w", corefactory.ErrConfigRequired)
)

// Builder returns the provider function of the nats type, ready for
// dlockfactory.DLockBuilder.UseProvider. The provider keeps locks in a
// JetStream KeyValue bucket created through conn on the Build context and
// uses the builder's logger.
func Builder(conn *nats.Conn) dlockfactory.ProviderFunc {
	return func(ctx context.Context, cfg *lockconfig.DistributionLock, logger *slog.Logger) (providers.Provider, error) {
		if cfg.NATS == nil {
			return nil, ErrConfigRequired
		}
		if conn == nil {
			return nil, ErrClientRequired
		}

		// Built here rather than through natsprovider.NewDLock, which takes no
		// provider options; the error wrapping matches it.
		provOpts := []natsprovider.Option{
			natsprovider.WithLogger(logger),
			natsprovider.WithBucket(cfg.NATS.Bucket),
			natsprovider.WithStorage(bucketStorage(cfg.NATS.Storage)),
		}
		provOpts = coreslices.AppendIf(provOpts, cfg.NATS.MigrateBucketTTL, natsprovider.WithMigrateBucketTTL())

		prov, err := natsprovider.New(ctx, conn, provOpts...)
		if err != nil {
			return nil, coreerrs.Provider("nats distributed lock", err)
		}
		return prov, nil
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

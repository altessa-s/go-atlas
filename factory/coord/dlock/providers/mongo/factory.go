// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/coord/dlock/providers"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	mongoprovider "github.com/altessa-s/go-atlas/coord/dlock/providers/mongo"
	dlockfactory "github.com/altessa-s/go-atlas/factory/coord/dlock"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

// ensureIndexesTimeout bounds the index setup Builder runs when the config
// asks for it, so a lock wait cannot stall startup indefinitely.
const ensureIndexesTimeout = 30 * time.Second

var (
	// ErrClientRequired is returned when the MongoDB lock provider is built
	// without a database.
	ErrClientRequired = errors.New("dlock: mongo database is required for provider mongo")
	// ErrConfigRequired is returned when the distribution lock configuration
	// has no mongo block.
	ErrConfigRequired = fmt.Errorf("dlock: mongo configuration is required for provider mongo: %w", corefactory.ErrConfigRequired)
)

// Builder returns the provider function of the mongo type, ready for
// dlockfactory.DLockBuilder.UseProvider. The provider keeps locks in a
// collection of db and logs through the builder's logger. With
// mongo.ensureIndexes set it reconciles the cleanup TTL index through the
// provider's idempotent EnsureIndexes, bounded by 30 seconds; otherwise it
// performs no I/O.
func Builder(db *mongodrv.Database) dlockfactory.ProviderFunc {
	return func(ctx context.Context, lockCfg *lockconfig.DistributionLock, logger *slog.Logger) (providers.Provider, error) {
		cfg := lockCfg.Mongo
		if cfg == nil {
			return nil, ErrConfigRequired
		}
		if db == nil {
			return nil, ErrClientRequired
		}
		provOpts := []mongoprovider.Option{mongoprovider.WithLogger(logger)}
		provOpts = coreslices.AppendIf(provOpts, cfg.Collection != "", mongoprovider.WithCollection(cfg.Collection))
		provOpts = coreslices.AppendIf(provOpts, cfg.TTL > 0, mongoprovider.WithTTL(cfg.TTL))
		provOpts = coreslices.AppendIf(provOpts, cfg.RenewRatio > 0, mongoprovider.WithRenewRatio(cfg.RenewRatio))
		provOpts = coreslices.AppendIf(provOpts, cfg.WithoutRenewal, mongoprovider.WithoutRenewal())
		provOpts = coreslices.AppendIf(provOpts, cfg.ExpiredCleanup, mongoprovider.WithExpiredCleanup(cfg.ExpiredCleanupGrace))
		prov, err := mongoprovider.New(db, provOpts...)
		if err != nil {
			return nil, coreerrs.Provider("mongodb distributed lock", err)
		}
		if cfg.EnsureIndexes {
			indexCtx, cancel := context.WithTimeout(ctx, ensureIndexesTimeout)
			defer cancel()
			if err := prov.EnsureIndexes(indexCtx); err != nil {
				return nil, coreerrs.Provider("mongodb distributed lock", err)
			}
		}
		return prov, nil
	}
}

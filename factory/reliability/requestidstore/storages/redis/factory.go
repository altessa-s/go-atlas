// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-aip/requestid"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	config "github.com/altessa-s/go-atlas/config/requestidstore"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	storefactory "github.com/altessa-s/go-atlas/factory/reliability/requestidstore"
	backend "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/redis"
)

// Builder binds the redis client for use with StoreBuilder.UseStorage.
func Builder(client redis.UniversalClient) storefactory.StorageFunc {
	return func(ctx context.Context, cfg *config.Config) (requestid.Store, error) {
		if nilcheck.IsNil(client) {
			return nil, corefactory.Errorf(corefactory.ErrDependencyRequired, "redis client required")
		}
		if cfg == nil || cfg.Redis == nil {
			return nil, corefactory.ErrConfigRequired
		}
		return backend.New(client, backend.WithKeyPrefix(cfg.Redis.KeysPrefix)), nil
	}
}

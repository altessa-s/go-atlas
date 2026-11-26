// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// BuildRegistered builds a non-memory limiter storage using its registered factory.
// cfg must be non-nil. limiter names the caller in missing-registration errors.
func BuildRegistered(
	ctx context.Context,
	cfg *limiterconfig.StorageConfig,
	builders map[limiterconfig.StorageType]StorageFunc,
	limiter string,
) (storages.Storage, error) {
	if build, ok := builders[cfg.Type]; ok {
		return build(ctx, cfg)
	}
	if cfg.Type == limiterconfig.StorageTypeRedis {
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "%s limiter storage %q is not registered: add factory/coord/limiters/storages/redis "+
			"with UseStorage(%q, redisfactory.Builder(client))", limiter, cfg.Type, cfg.Type)
	}
	return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported storage type: %s", cfg.Type)
}

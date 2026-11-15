// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// BuildRegistered builds a non-memory limiter storage using its registered factory.
// cfg must be non-nil. limiter names the caller in missing-registration errors.
func BuildRegistered(ctx context.Context, cfg *storageconfig.BackendConfig, contention *limiterconfig.NATSContention,
	builders map[storageconfig.BackendType]StorageFunc, limiter string) (storages.Storage, error) {
	if build, ok := builders[cfg.Type]; ok {
		return build(ctx, cfg, contention)
	}
	switch typ := cfg.Type; typ {
	case storageconfig.BackendTypeRedis:
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "%s limiter storage %q is not registered: add factory/coord/limiters/storages/redis "+
			"with UseStorage(%q, redisfactory.Builder(client))", limiter, typ, typ)
	case storageconfig.BackendTypeNATS:
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "%s limiter storage %q is not registered: add factory/coord/limiters/storages/nats "+
			"with UseStorage(%q, natsfactory.Builder(js))", limiter, typ, typ)
	default:
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported storage type: %s", typ)
	}
}

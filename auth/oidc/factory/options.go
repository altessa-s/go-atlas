// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/auth/oidc"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// UseLogger sets the logger for the builder and all created components.
func (b *ProviderBuilder) UseLogger(v *slog.Logger) *ProviderBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ProviderBuilder) UseDefaultLogger() *ProviderBuilder {
	return b.UseLogger(slog.Default())
}

// UseScheduler sets the task registrar used for background JWKS refresh
// and revocation sync tasks.
func (b *ProviderBuilder) UseScheduler(v corescheduler.TaskRegistrar) *ProviderBuilder {
	b.scheduler = v
	return b
}

// UseTokenCache sets the cache used for validated token caching.
func (b *ProviderBuilder) UseTokenCache(v oidc.Cacher) *ProviderBuilder {
	b.tokenCache = v
	return b
}

// UseRedisClient sets the Redis client used for revocation filter storage.
// Required only when revocation is enabled and no custom [oidc.RevocationStorage]
// is provided via [ProviderBuilder.UseRevocationStorage].
func (b *ProviderBuilder) UseRedisClient(v redis.UniversalClient) *ProviderBuilder {
	b.redisClient = v
	return b
}

// UseRevocationStorage sets a custom revocation storage, bypassing automatic
// creation from config. When set, [ProviderBuilder.UseRedisClient] is not required
// for revocation.
func (b *ProviderBuilder) UseRevocationStorage(v oidc.RevocationStorage) *ProviderBuilder {
	b.revocationStorage = v
	return b
}

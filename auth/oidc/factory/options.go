// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/auth/oidc"
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

// UseRevocationAuthoritative sets the exact revocation store used to confirm
// probabilistic filter hits. Without it a filter hit is reported as revoked on
// its own, so the Bloom false-positive rate (1% by default) becomes a rate of
// rejected valid tokens.
//
// The store MUST hold the same revocation set the filter is built from
// (config.OIDCRevocation.Source); confirming against an unrelated store turns
// every hit into "not revoked" and silently disables revocation.
func (b *ProviderBuilder) UseRevocationAuthoritative(v oidc.Authoritative) *ProviderBuilder {
	b.revocationAuthoritative = v
	return b
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
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

// UseMeter sets the meter used to record metrics for the OIDC provider.
func (b *ProviderBuilder) UseMeter(v metrics.Meter) *ProviderBuilder {
	b.meter = v
	return b
}

// UseTokenCache sets the cache used for validated token caching.
func (b *ProviderBuilder) UseTokenCache(v oidc.Cacher) *ProviderBuilder {
	b.tokenCache = v
	return b
}

// UseRevocationBloomStorage registers the Bloom storage function of one
// storage type for the revocation filter, passed through to the probfilter
// factory (FilterBuilder.UseBloomStorage). It is required only when
// revocation.filter selects a Bloom filter with the redis storage type and no
// custom [oidc.RevocationStorage] is set: register
// factory/cache/probfilter/bloom/storages/redis.Builder(client). A later
// registration of the same type replaces the earlier one.
func (b *ProviderBuilder) UseRevocationBloomStorage(
	typ probfilterconfig.StorageType, build probfilterfactory.BloomStorageFunc,
) *ProviderBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseRevocationBloomStorage %q: nil function", typ))
		return b
	}
	if b.bloomStorages == nil {
		b.bloomStorages = map[probfilterconfig.StorageType]probfilterfactory.BloomStorageFunc{}
	}
	b.bloomStorages[typ] = build
	return b
}

// UseRevocationCuckooStorage registers the Cuckoo storage function of one
// storage type for the revocation filter, passed through to the probfilter
// factory (FilterBuilder.UseCuckooStorage). It is required only when
// revocation.filter selects a Cuckoo filter with the redis storage type and
// no custom [oidc.RevocationStorage] is set: register
// factory/cache/probfilter/cuckoo/storages/redis.Builder(client). A later
// registration of the same type replaces the earlier one.
func (b *ProviderBuilder) UseRevocationCuckooStorage(
	typ probfilterconfig.StorageType, build probfilterfactory.CuckooStorageFunc,
) *ProviderBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseRevocationCuckooStorage %q: nil function", typ))
		return b
	}
	if b.cuckooStorages == nil {
		b.cuckooStorages = map[probfilterconfig.StorageType]probfilterfactory.CuckooStorageFunc{}
	}
	b.cuckooStorages[typ] = build
	return b
}

// UseRevocationStorage sets a custom revocation storage, bypassing automatic
// creation from config.
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
// (authconfig.OIDCRevocation.Source); confirming against an unrelated store turns
// every hit into "not revoked" and silently disables revocation.
func (b *ProviderBuilder) UseRevocationAuthoritative(v oidc.Authoritative) *ProviderBuilder {
	b.revocationAuthoritative = v
	return b
}

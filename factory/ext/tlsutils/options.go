// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-tlsutils"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *ProvidersBuilder) UseLogger(v *slog.Logger) *ProvidersBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ProvidersBuilder) UseDefaultLogger() *ProvidersBuilder {
	return b.UseLogger(slog.Default())
}

// UseOcspStapler sets the OCSP stapler for TLS providers.
func (b *ProvidersBuilder) UseOcspStapler(v tlsutils.OCSPStapler) *ProvidersBuilder {
	b.ocspStapler = v
	return b
}

// UseProvider registers the provider builder of one TLS provider type
// (tlsproviders.ProviderTypeVault, tlsproviders.ProviderTypeS3). A later
// registration of the same type replaces the earlier one; the file and
// Let's Encrypt providers are built in and cannot be replaced.
func (b *ProvidersBuilder) UseProvider(typ tlsproviders.ProviderType, build ProviderBuilder) *ProvidersBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseProvider %q: nil builder", typ))
		return b
	}
	if b.providers == nil {
		b.providers = map[tlsproviders.ProviderType]ProviderBuilder{}
	}
	b.providers[typ] = build
	return b
}

// UseCacheDir sets the cache directory for TLS certificate caching.
func (b *ProvidersBuilder) UseCacheDir(v string) *ProvidersBuilder {
	b.cacheDir = v
	return b
}

// UseScheduler injects a TaskRegistrar used by [ProvidersBuilder.Build]
// to register periodic OCSP refresh. Required only when the YAML config
// sets a non-empty tlsProvider.ocsp.refreshSchedule and no stapler was
// already injected via [ProvidersBuilder.UseOcspStapler]. When omitted
// the auto-constructed stapler still works — entries refresh lazily on
// the first cache miss after expiration — but fresh handshakes can
// block on an OCSP fetch.
func (b *ProvidersBuilder) UseScheduler(v schedtask.Registrar) *ProvidersBuilder {
	b.scheduler = v
	return b
}

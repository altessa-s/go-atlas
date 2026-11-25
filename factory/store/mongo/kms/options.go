// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"log/slog"
	"maps"
	"slices"

	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *ProviderBuilder) UseLogger(v *slog.Logger) *ProviderBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ProviderBuilder) UseDefaultLogger() *ProviderBuilder {
	return b.UseLogger(slog.Default())
}

// UseTlsConfig sets the TLS configuration for the KMS provider connection.
func (b *ProviderBuilder) UseTlsConfig(v *tls.Config) *ProviderBuilder {
	b.tlsConfig = v
	return b
}

// UseProvider registers a custom KMS provider. Built-in providers retain their constructors.
func (b *ProviderBuilder) UseProvider(typ mongoconfig.KMSProvider, build ProviderFunc) *ProviderBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(corefactory.ErrDependencyRequired, "UseProvider %q: nil function", typ))
		return b
	}
	if b.providers == nil {
		b.providers = make(map[mongoconfig.KMSProvider]ProviderFunc)
	}
	b.providers[typ] = build
	return b
}

// RegisteredProviderTypes returns a snapshot of the names registered with UseProvider for standalone configuration validation.
func (b *ProviderBuilder) RegisteredProviderTypes() []mongoconfig.KMSProvider {
	return slices.Sorted(maps.Keys(b.providers))
}

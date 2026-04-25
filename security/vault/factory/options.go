// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *VaultBuilder) UseLogger(v *slog.Logger) *VaultBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *VaultBuilder) UseDefaultLogger() *VaultBuilder {
	return b.UseLogger(slog.Default())
}

// UseTlsConfig sets the TLS configuration for the Vault client.
func (b *VaultBuilder) UseTlsConfig(v *tls.Config) *VaultBuilder {
	b.tlsConfig = v
	return b
}

// UseHealthCoordinator sets the health coordinator for the Vault client.
func (b *VaultBuilder) UseHealthCoordinator(v *health.Coordinator) *VaultBuilder {
	b.healthCoordinator = v
	return b
}

// UseCollector sets the [metrics.Collector] for recording Vault metrics.
func (b *VaultBuilder) UseCollector(v metrics.Collector) *VaultBuilder {
	b.collector = v
	return b
}

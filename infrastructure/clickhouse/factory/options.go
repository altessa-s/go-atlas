// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *ClickHouseBuilder) UseLogger(v *slog.Logger) *ClickHouseBuilder {
	b.SetLogger(v)

	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ClickHouseBuilder) UseDefaultLogger() *ClickHouseBuilder {
	return b.UseLogger(slog.Default())
}

// UseTlsConfig sets the TLS configuration for the ClickHouse connection.
// It overrides any TLS carried by a connection DSN.
func (b *ClickHouseBuilder) UseTlsConfig(v *tls.Config) *ClickHouseBuilder {
	b.tlsConfig = v

	return b
}

// UseHealthCoordinator sets the health coordinator used to register a
// health checker for the created connection.
func (b *ClickHouseBuilder) UseHealthCoordinator(v *health.Coordinator) *ClickHouseBuilder {
	b.healthCoordinator = v

	return b
}

// UseHealthServiceName sets the service name used when registering the
// health checker with the coordinator. Defaults to
// [DefaultHealthServiceName].
func (b *ClickHouseBuilder) UseHealthServiceName(v string) *ClickHouseBuilder {
	b.healthServiceName = v

	return b
}

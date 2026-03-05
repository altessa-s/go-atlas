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
func (b *ConnectionBuilder) UseLogger(v *slog.Logger) *ConnectionBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ConnectionBuilder) UseDefaultLogger() *ConnectionBuilder {
	return b.UseLogger(slog.Default())
}

// UseTlsConfig sets the TLS configuration for the NATS connection.
func (b *ConnectionBuilder) UseTlsConfig(v *tls.Config) *ConnectionBuilder {
	b.tlsConfig = v
	return b
}

// UseHealthCoordinator sets the health coordinator used to register a
// health checker for the created NATS connection.
func (b *ConnectionBuilder) UseHealthCoordinator(v *health.Coordinator) *ConnectionBuilder {
	b.healthCoordinator = v
	return b
}

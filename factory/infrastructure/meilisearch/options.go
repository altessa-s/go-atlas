// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and the created client.
func (b *ClientBuilder) UseLogger(v *slog.Logger) *ClientBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ClientBuilder) UseDefaultLogger() *ClientBuilder {
	return b.UseLogger(slog.Default())
}

// UseHealthCoordinator sets the health coordinator used to register a
// health checker for the created Meilisearch client.
func (b *ClientBuilder) UseHealthCoordinator(v *health.Coordinator) *ClientBuilder {
	b.healthCoordinator = v
	return b
}

// UseHealthServiceName sets the service name used when registering the
// health checker with the coordinator. Defaults to "meilisearch" if not set.
func (b *ClientBuilder) UseHealthServiceName(v string) *ClientBuilder {
	b.healthServiceName = v
	return b
}

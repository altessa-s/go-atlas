// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"log/slog"

	"github.com/altessa-s/go-atlas/data/mongo/kms"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *MongoBuilder) UseLogger(v *slog.Logger) *MongoBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *MongoBuilder) UseDefaultLogger() *MongoBuilder {
	return b.UseLogger(slog.Default())
}

// UseTlsConfig sets the TLS configuration for the MongoDB connection.
func (b *MongoBuilder) UseTlsConfig(v *tls.Config) *MongoBuilder {
	b.tlsConfig = v
	return b
}

// UseKmsProvider sets the KMS provider used for client-side field level encryption.
func (b *MongoBuilder) UseKmsProvider(v kms.Provider) *MongoBuilder {
	b.kmsProvider = v
	return b
}

// UseHealthCoordinator sets the health coordinator used to register a
// health checker for the created MongoDB client.
func (b *MongoBuilder) UseHealthCoordinator(v *health.Coordinator) *MongoBuilder {
	b.healthCoordinator = v
	return b
}

// UseHealthServiceName sets the service name used when registering the
// health checker with the coordinator. Defaults to "mongo" if not set.
func (b *MongoBuilder) UseHealthServiceName(v string) *MongoBuilder {
	b.healthServiceName = v
	return b
}

// UseCollector sets the [metrics.Collector] for recording MongoDB metrics.
func (b *MongoBuilder) UseCollector(v metrics.Collector) *MongoBuilder {
	b.collector = v
	return b
}

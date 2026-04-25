// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"log/slog"
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

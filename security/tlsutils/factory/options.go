// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/security/tlsutils"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	tlss3 "github.com/altessa-s/go-atlas/security/tlsutils/providers/s3"
	vaultApi "github.com/hashicorp/vault/api"
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

// UseVaultClient sets the Vault client for the Vault TLS provider.
func (b *ProvidersBuilder) UseVaultClient(v *vaultApi.Client) *ProvidersBuilder {
	b.vaultClient = v
	return b
}

// UseCacheDir sets the cache directory for TLS certificate caching.
func (b *ProvidersBuilder) UseCacheDir(v string) *ProvidersBuilder {
	b.cacheDir = v
	return b
}

// UseS3Client sets the S3 client for the S3 TLS provider.
func (b *ProvidersBuilder) UseS3Client(v tlss3.S3API) *ProvidersBuilder {
	b.s3Client = v
	return b
}

// UseScheduler injects a TaskRegistrar used by [ProvidersBuilder.Build]
// to register periodic OCSP refresh. Required only when the YAML config
// sets a non-empty tlsProvider.ocsp.refreshSchedule and no stapler was
// already injected via [ProvidersBuilder.UseOcspStapler]. When omitted
// the auto-constructed stapler still works — entries refresh lazily on
// the first cache miss after expiration — but fresh handshakes can
// block on an OCSP fetch.
func (b *ProvidersBuilder) UseScheduler(v corescheduler.TaskRegistrar) *ProvidersBuilder {
	b.scheduler = v
	return b
}

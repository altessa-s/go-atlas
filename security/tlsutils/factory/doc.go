// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating TLS configurations and providers
// from configuration.
//
// [ProvidersBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ProvidersBuilder.Build] time.
//
//	providers, err := factory.New(cfg.TlsProvider).
//	    UseLogger(logger).
//	    UseVaultClient(vaultClient).
//	    UseCacheDir("./certs").
//	    Build()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer providers.Close(ctx, nil)
//
// For client TLS configurations, use [ProvidersBuilder.CreateClientConfig]:
//
//	b := factory.New(nil).UseLogger(logger)
//	tlsConfig, err := b.CreateClientConfig(cfg.TlsClient)
package factory

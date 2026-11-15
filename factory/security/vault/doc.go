// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating Vault clients
// from configuration.
//
// [VaultBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [VaultBuilder.Build] time.
//
//	v, err := factory.New(cfg.Vault).
//	    UseLogger(logger).
//	    UseTlsConfig(tlsConfig).
//	    Build(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer v.StopRenewal()
package factory

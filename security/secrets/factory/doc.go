// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating secrets managers and providers
// from configuration. It integrates with Vault, GCP Secret Manager, Yandex Cloud Lockbox,
// and in-memory storage implementations.
//
// [ManagerBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [ManagerBuilder.Build] time.
//
//	manager, err := factory.New(&cfg.Secrets).
//	    UseLogger(logger).
//	    UseScheduler(sched).
//	    UseVaultClient(vaultClient).
//	    Build(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer manager.Shutdown()
package factory

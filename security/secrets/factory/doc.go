// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of secrets managers and providers.
// It integrates with Vault, GCP Secret Manager, Yandex Cloud Lockbox, and in-memory
// storage implementations to create Manager instances with the appropriate provider backend.
//
// Example using configuration:
//
//	f := factory.New(
//	    factory.WithLogger(logger),
//	    factory.WithScheduler(sched),
//	)
//	manager, err := f.CreateManagerFromConfig(ctx, &cfg.Secrets, vaultClient)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer manager.Shutdown()
package factory

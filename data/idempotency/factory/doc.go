// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of idempotency storage.
// It integrates idempotency Keeper with memory, Redis, and NATS storage backends
// to create storage instances with consistent defaults.
//
// Example:
//
//	f := factory.New(factory.WithLogger(logger))
//	keeper, err := f.CreateKeeperFromConfig(cfg.Idempotency)
//	if err != nil {
//		log.Fatal(err)
//	}
package factory

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of rate limiters.
// It integrates token bucket limiter with memory, Redis, and NATS storage backends
// to create rate limiters with consistent defaults.
//
// Example:
//
//	f := factory.New(factory.WithLogger(logger))
//	storage, err := f.CreateStorageFromConfig(cfg.Storage)
//	if err != nil {
//		log.Fatal(err)
//	}
//	limiter, err := f.CreateLimiterFromConfig(cfg.Limiter, storage)
//	if err != nil {
//		log.Fatal(err)
//	}
package factory

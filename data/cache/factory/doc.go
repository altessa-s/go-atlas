// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of cache instances and providers.
//
// The factory supports creating various cache providers from configuration:
//   - Redis: distributed caching with Redis
//   - FreeCache: high-performance in-memory caching (memory type)
//   - Noop: no-op provider for testing
//
// Example usage:
//
//	f := factory.New(
//		factory.WithLogger(logger),
//		factory.WithRedisClient(redisClient),
//	)
//
//	// Create provider from configuration
//	provider, err := f.CreateProviderFromConfig(cfg.Cache)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Or create no-op cache for testing
//	noopCache := f.CreateNoopCache()
package factory

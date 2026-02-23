// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of unique value managers.
// It integrates uniq implementations with Redis and NATS backends
// to create unique value managers with consistent defaults.
//
// Example:
//
//	f := factory.New(factory.WithRedisClient(redisClient))
//	u, err := f.CreateUniqFromConfig(cfg)
package factory

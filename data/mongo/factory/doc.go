// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating cursor storages
// from configuration.
//
// [CursorStorageBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [CursorStorageBuilder.Build] time.
//
//	storage, err := factory.New(cfg.Storage).
//	    UseLogger(logger).
//	    UseRedisClient(redisClient).
//	    WithTTL(time.Hour).
//	    Build(ctx)
package factory

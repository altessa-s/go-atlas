// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides a Redis-backed implementation of the [scheduler.Storage] interface.
// It stores task states and execution history as JSON documents using RedisJSON,
// and uses RediSearch for indexed querying and filtering.
//
// [Storage] is safe for concurrent use by multiple goroutines and across multiple
// processes, so several scheduler instances can share state.
//
// # Prerequisites
//
// The Redis server must have the RedisJSON and RediSearch modules loaded.
// Call [Storage.EnsureIndexes] once during application startup to create the
// required RediSearch indexes idempotently.
//
// # Key patterns
//
//   - Task state: {prefix}:task:{task_id}
//   - History entry: {prefix}:history:{task_id}:{history_id}
//   - Task index: {prefix}:idx:tasks (RediSearch FT index)
//   - History index: {prefix}:idx:history (RediSearch FT index)
//
// The default key prefix is [DefaultKeyPrefix] ("scheduler") and can be
// overridden with [WithKeyPrefix].
package redis

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redisutils provides common utilities for Redis-based data providers.
// It standardizes key building with consistent separator handling.
//
// Example:
//
//	kb := redisutils.NewKeyBuilder("myapp:cache")
//	key := kb.Build("user:123") // returns "myapp:cache:user:123"
package redisutils

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package helpers provides low-level runtime utilities for debugging and tracing.
//
// All functions are thread-safe and can be called from any goroutine.
//
// # Usage
//
//	// Get current goroutine ID
//	id := helpers.GoroutineID()
//	log.Printf("Running in goroutine %d", id)
//
// # Performance
//
// GoroutineID() parses the stack trace output, which has some overhead.
// Use sparingly in hot paths.
package helpers

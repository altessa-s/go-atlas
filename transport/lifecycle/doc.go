// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package lifecycle provides timeout settings shared by HTTP and gRPC servers.
//
// # Configuration
//
// Start with [DefaultTimeouts] to customize one duration.
//
//	limits := lifecycle.DefaultTimeouts()
//	limits.ShutdownGraceful = 30 * time.Second
package lifecycle

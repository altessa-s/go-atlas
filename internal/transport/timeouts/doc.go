// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package timeouts defines the timeout parameters that govern transport
// server lifecycle: startup verification, graceful shutdown deadline, and
// shutdown-progress warning interval.
//
// [Default] returns production-ready defaults. Override individual fields
// when tighter or looser bounds are needed:
//
//	cfg := timeouts.Default()
//	cfg.ShutdownGraceful = 30 * time.Second
package timeouts

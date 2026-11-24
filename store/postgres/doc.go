// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package postgres opens bounded database/sql pools over pgx and carries application
// transactions through context. [Querier] routes each call through its transaction
// or named pool. [Migrate] applies ordered forward migrations under a schema lock.
// [Open] accepts observability dependencies through functional options.
package postgres

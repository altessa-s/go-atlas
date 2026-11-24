// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package postgres opens bounded database/sql pools over pgx and carries application
// transactions through context. [Querier] routes each call through its transaction
// or named pool.
// [NewCommitter] adapts [WithTransaction] for eventbus units of work. [Transaction] detects foreign write transactions. [Committer.Detect]
// identifies the selected pool's open write transaction. [WithTxRetries] enables whole-body
// replay for replay-safe roots. Cross-pool nesting and independent commits in replayable
// bodies are rejected. Transaction completion belongs to the owner.
// [Open] accepts observability dependencies through functional options.
package postgres

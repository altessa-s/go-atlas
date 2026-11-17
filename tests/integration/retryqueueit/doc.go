// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package retryqueueit runs the reliability/retryqueue MongoDB storage against the
// compose replica set and the SQL storage against PostgreSQL, MariaDB and
// MySQL: the storage contract suite from
// reliability/retryqueue/storages/storagetest, then what only a real server shows.
//
//   - Indexes. EnsureIndexes creates exactly the due-list, per-state and
//     (with a retention) TTL indexes, idempotently, and the TTL index purges
//     only resolved items.
//   - Query plans. The due list and the per-state list are served by an index
//     scan with the sort taken from the index — no collection scan and no
//     in-memory sort as the queue grows.
//   - Replicas. Several stores over one collection or table, as several
//     service replicas run them, race the first Mark of a key and still agree
//     on one item.
//   - SQL retention and transactions. PurgeExpired deletes resolved items
//     past the retention by the database clock, in chunks; Move joins the
//     transaction WithTx carries.
//
// Tests skip rather than fail when a server is unreachable. See
// tests/integration/README.md.
//
// # Purging
//
// SQL PurgeExpired(ctx) returns an int64 total across bounded batches, including completed batches
// before an error. Retention options enable purging; a zero purge interval disables only background
// work.
package retryqueueit

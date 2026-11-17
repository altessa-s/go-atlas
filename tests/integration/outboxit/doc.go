// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package outboxit exercises reliability/outbox against a live MongoDB replica set and,
// for every scenario that does not depend on change streams, against the SQL
// store on live PostgreSQL, MariaDB and MySQL servers. The storage contract
// suite (reliability/outbox/storages/storagetest) runs on all four, and its
// Watch contract on MongoDB change streams and on PostgreSQL LISTEN/NOTIFY
// through [PGListener], the pgx adapter of the SQL store's Listener.
//
// The unit tests for reliability/outbox run against in-memory stores and assert on the
// state the outbox decided to write. What they cannot assert is whether that
// state survives a real store: the batch fetch takes a lock inside a
// transaction, every deadline is evaluated against the server clock rather than
// the caller's, and writes are fenced by a lock token. None of those mechanisms
// exists until a real MongoDB is on the other side.
//
// Four properties in particular only appear here:
//
//   - Atomicity. Save shares the caller's transaction, so an aborted business
//     write must leave no event behind and a committed one must leave exactly
//     one. This is the entire reason the pattern exists, and an in-memory store
//     cannot show it.
//
//   - Exclusivity under contention. Several dispatchers polling one collection
//     must not hand the same event to two handlers, and a dispatcher whose lock
//     was reclaimed must not overwrite the result of the one that took over.
//
//   - Durable backoff. A failed event becomes eligible again only after its
//     backoff has elapsed on the server's clock, and a dead-lettered one is
//     retained rather than swept up with the successfully published events.
//
//   - Change notifications. Outbox.Watch dispatches without any poll cycle at
//     all, and only at commit — an uncommitted insert is invisible to a change
//     stream, and pg_notify inside the saving transaction is delivered on
//     commit, which is what makes them safe to drive delivery from. Neither
//     exists without the real server (an oplog, a LISTEN connection).
//
// For MongoDB a replica set is required — transactions do not exist on a
// standalone mongod.
// Tests skip rather than fail when none is reachable, so a machine without the
// compose stack still gets a green build. See tests/integration/README.md.
package outboxit

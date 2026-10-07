// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package auditit runs the data/audit storage conformance suite against the
// SQL storage on live PostgreSQL, MariaDB and MySQL servers, each contract over
// its own throwaway table, and checks what only a real server shows: a batch
// is atomic, and a replayed batch is accepted without duplicating events.
//
// The unit tests of data/audit/storages/sqldb check statement shape against a
// fake driver; whether (timestamp millisecond, ID) ordering is byte-wise on a
// latin1 MySQL, or the conflict clauses keep a retried batch idempotent, only
// shows here.
//
// Tests skip rather than fail when a server is unreachable. See
// tests/integration/README.md.
package auditit

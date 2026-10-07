// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sagait runs the data/saga storage contract suite against live
// backends: the SQL store on PostgreSQL, MariaDB and MySQL, and the MongoDB and
// Redis stores, each contract over its own throwaway table, database or key
// prefix. It also drives an orchestrator end to end over each SQL server.
//
// The unit tests of data/saga/storages/sqldb check statement shape and error
// mapping against a fake driver; whether insert-if-absent, the version
// compare-and-swap and the recovery predicate hold on a real server — under
// concurrent writers, with binary columns on a latin1 MySQL — only shows here.
//
// Tests skip rather than fail when a server is unreachable, so a machine
// without the compose stack still gets a green build. The MongoDB contracts
// run only when MONGO_URI is set, so the suite never writes to an unrelated
// server listening on the compose port. See tests/integration/README.md.
package sagait

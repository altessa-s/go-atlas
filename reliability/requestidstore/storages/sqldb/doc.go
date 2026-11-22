// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldb stores go-aip requestid reservations in PostgreSQL, MySQL or MariaDB.
//
// Each operation checks ExpiresAt against the caller's now. Configure [WithCleanupGrace]
// above the maximum skew between callers so a leading clock cannot reclaim a live entry.
//
// # Usage
//
//	store, err := sqldb.New(db, sqldb.DialectPostgres)
//	if err != nil { return err }
//	if err := store.EnsureSchema(ctx); err != nil { return err }
//	guard, err := requestid.New(store)
//
// Stop the background collector with [Storage.Close] when the service shuts down.
package sqldb

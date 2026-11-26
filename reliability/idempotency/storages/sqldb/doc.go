// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldb implements idempotency storage on PostgreSQL, MySQL and MariaDB
// through database/sql. The caller supplies the database and registers a driver.
//
// # Schema and expiry
//
// New performs no I/O and starts no goroutine; call Storage.EnsureSchema at startup or use migrations.
// Expiry uses the database clock. Storage.PurgeExpired provides bounded physical
// cleanup. WithPurgeInterval enables a lazy background sweep; Close stops it.
// EnsureSchema creates the composite (expires_at, key) index with suffix
// expires_key on both dialects.
//
// # Transactions
//
// WithTx makes key operations join the caller's business transaction. The caller
// owns commit, rollback and retry. ErrRestartTransaction marks deadlocks,
// serialization failures and MariaDB stale snapshots that require a full retry.
// Complete and Release compare live stored bytes. Steal also checks locked_at
// against the database clock. A nil Complete token is an unconditional write.
//
// # Usage
//
//	store, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTTL(24*time.Hour))
//	if err != nil {
//	    return err
//	}
//	return store.EnsureSchema(ctx)
//
// # Purging
//
// PurgeExpired(ctx) (int64, error) drains currently purgeable rows in batches and returns completed
// deletions even if a later batch fails. WithPurgeInterval defaults to one minute; zero disables the
// background loop while manual purging stays available. WithPurgeBatchSize defaults to 1000. New
// starts no goroutine; the first write starts the loop for all keys. Close(ctx) permanently stops it
// and waits within the context, including on repeated calls. A loop that has already exited satisfies
// an ended context. Each statement is bounded by a 30-second timeout.
// PostgreSQL uses FOR UPDATE SKIP LOCKED. MySQL selects keys without locking,
// then deletes by key while rechecking expiry; draining follows selected rows
// so concurrent renewals cannot truncate a pass. Explicit purges join WithTx;
// background contexts carry no request values or transactions. Background failures and recovered panics go to WithLogger (discard by default).
package sqldb

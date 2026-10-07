// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldb implements a [providers.Provider] for distributed locks on a
// SQL database through the standard database/sql package: PostgreSQL 12+
// ([DialectPostgres]) and MySQL 8.0+ / MariaDB 10.6+ ([DialectMySQL]), for
// deployments that coordinate replicas through a relational database rather
// than NATS or MongoDB. The caller owns the *sql.DB and chooses the driver.
//
// # Model
//
// Each lock key is one row of a table (default "dlocks"). A lock is taken only
// while the key has no unexpired lease: on PostgreSQL by one INSERT … ON
// CONFLICT DO UPDATE … WHERE expired, on MySQL by creating the row on first
// use and then a conditional UPDATE in a transaction that also reads the new
// token. Every lease decision compares against the database clock in Unix
// microseconds — clock_timestamp() on PostgreSQL, so a decision taken after a
// lock wait sees the actual time, and UTC_TIMESTAMP(6) on MySQL — so replicas
// with skewed clocks agree. Each acquisition increments the row's fencing
// token, exposed as [providers.LockInfo.FencingToken]; releasing a lock ends
// its lease but keeps the row, so tokens never restart.
//
// While the lock's context lives, the lease is renewed every TTL × renew
// ratio (default 10 s × 1/3). Renewal locks the row and then, in the next
// statement of the same transaction, requires the holder, its fencing token
// and an unexpired lease, so the clock is read after any lock wait: an expired
// lease is never revived, and a stale holder can neither renew nor release the
// new holder's lease. Ending the
// context releases the lease. An acquisition whose reply arrives after the TTL
// or that fails ambiguously is released by its unique owner id, exactly as the
// MongoDB provider does.
//
// # Schema
//
// [New] performs no I/O. Call [Locker.EnsureSchema] once at startup — it is
// idempotent — or apply the same DDL through a migration tool. Keys compare
// exactly; on MySQL/MariaDB the key and owner are binary columns.
//
// # Usage
//
//	locker, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTTL(15*time.Second))
//	if err != nil {
//	    return err
//	}
//	if err := locker.EnsureSchema(ctx); err != nil {
//	    return err
//	}
//	lk, err := locker.Lock(ctx, "ticket:42")
//	if errors.Is(err, errs.ErrLockNotHeld) {
//	    // another replica holds it
//	}
//	defer lk.Release(context.Background())
package sqldb

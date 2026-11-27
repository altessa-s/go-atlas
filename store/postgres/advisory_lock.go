// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"hash/fnv"
)

const advisoryLockNamespace = "go-atlas/postgres:"

// ErrWriteTransactionRequired indicates that the context lacks an open writable transaction.
var ErrWriteTransactionRequired = errors.New("open write transaction required")

// AdvisoryLock acquires an exclusive PostgreSQL transaction-level advisory lock
// for key, waiting if another transaction holds it. It returns
// [ErrWriteTransactionRequired] before issuing SQL unless ctx belongs to an
// open writable transaction. PostgreSQL releases the lock on commit or
// rollback, including rollback to a savepoint established before acquisition.
func AdvisoryLock(ctx context.Context, key string) error {
	t := attached(ctx)
	if t == nil || t.readOnly || !t.open() {
		return ErrWriteTransactionRequired
	}
	_, err := t.tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", AdvisoryLockKey(key))
	return diagnostic("acquire advisory lock", err)
}

// AdvisoryLockKey returns the stable PostgreSQL bigint key used by [AdvisoryLock].
// It hashes the bytes of "go-atlas/postgres:" followed by the unchanged bytes
// of key using FNV-1a 64-bit, then interprets the result as a signed int64.
// The namespace and hash are fixed across releases.
func AdvisoryLockKey(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(advisoryLockNamespace + key))
	return int64(h.Sum64()) // Interpret the hash's bits as PostgreSQL's signed bigint.
}

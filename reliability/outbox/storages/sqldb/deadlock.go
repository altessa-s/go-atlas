// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/internal/sqldialect"

	coreretry "github.com/altessa-s/go-atlas-core/retry"
)

// Deadlock retry bounds. The statements that can meet on the same rows —
// [Store.UpdateEvents] and [Store.UnlockStuckEvents] — lock them in primary-key
// order, so a deadlock should not arise between them; the retry covers what the
// ordering cannot, such as locks the database takes on index entries.
const (
	// deadlockRetries is the number of retries after the first attempt.
	deadlockRetries = 3

	// deadlockBaseDelay is the pause before the first retry.
	deadlockBaseDelay = 10 * time.Millisecond

	// deadlockMaxDelay caps the pause between retries.
	deadlockMaxDelay = 200 * time.Millisecond

	// deadlockJitter spreads the retries of the transactions that collided.
	deadlockJitter = 0.5
)

// Server error codes the store recognizes.
const (
	// PostgreSQL SQLSTATEs: deadlock_detected and serialization_failure. MySQL
	// reports a deadlock under the latter as well.
	sqlStateDeadlock      = "40P01"
	sqlStateSerialization = "40001"

	// MySQL error number: ER_LOCK_DEADLOCK.
	mysqlDeadlock = 1213
)

// deadlockRetry re-runs a statement or transaction the database aborted to
// break a deadlock. Both dialects roll the victim back, so a retry starts clean.
var deadlockRetry = coreretry.NewPolicy(
	coreretry.WithMaxAttempts(deadlockRetries),
	coreretry.WithShouldRetry(isDeadlock),
	coreretry.WithNextDelay(coreretry.Exponential(
		deadlockBaseDelay,
		coreretry.WithExponentialMaxDelay(deadlockMaxDelay),
		coreretry.WithExponentialJitter(deadlockJitter),
	)),
)

// sqlStateError is the shape of the PostgreSQL drivers' server error.
type sqlStateError interface {
	error
	SQLState() string
}

// isDeadlock reports whether err is the database aborting a transaction to
// break a deadlock or a serialization conflict.
//
// The package imports no SQL driver, so the code is read by shape: pgx and
// lib/pq expose the SQLSTATE through a SQLState method, go-sql-driver/mysql
// exposes the server error number only as the Number field of its error type.
func isDeadlock(err error) bool {
	if e, ok := errors.AsType[sqlStateError](err); ok {
		state := e.SQLState()
		return state == sqlStateDeadlock || state == sqlStateSerialization
	}
	return sqldialect.MySQLErrorNumber(err) == mysqlDeadlock
}

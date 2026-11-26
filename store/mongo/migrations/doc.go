// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package migrations registers and runs MongoDB schema migrations under a database lease.
//
// # Ownership
//
// [WithRegistry] selects an independent set of migrations. By default [Run] merges init registrations with the mongo-migrate registry.
// Lease release checks the owner and uses a bounded cleanup context. The lease is renewed until the work returns; a failed renewal
// cancels the work. [WithRollbackOnFailure] reverts only the steps applied by the failing run, newest first, while the lease is held,
// and stops at a step registered without a down with [ErrIrreversibleMigration].
//
// # Partial migration failures
//
// Register a step with [WithCleanupOnFailure] to compensate for partial changes if its up returns an error. Cleanup runs on a fresh,
// bounded context under the verified lease and before any rollback of earlier steps. The failed step's down is never called.
// The failed version stays pending. Both up and cleanup errors are available through errors.Is.
//
// # Usage
//
//	err := migrations.Run(ctx, db)
package migrations

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package migrations

import "log/slog"

//go:generate go tool optgen generate
//go:generate go tool optgen generate --type=migrationOptions --option-type=MigrationOption --output=migration_options_gen.go

type migrationOptions struct {
	// WithCleanupOnFailure registers a handler that compensates for partial changes when this step's Up returns an error.
	// The handler must tolerate any partial state Up can leave, including no changes, and be safe to repeat across retries.
	// Limit it to changes from this step and honor the context, which is bounded by [MigrationTimeout] and canceled on lease loss.
	// A nil handler leaves partial changes for a later retry or manual repair.
	cleanupOnFailure MigrationFunc
}

type options struct {
	// WithRegistry selects an independent set of migrations for this run.
	registry *Registry

	// rollbackOnFailure rolls back the migrations applied by this run when an up step fails.
	rollbackOnFailure bool
	// logger receives rollback and lease-release failures.
	logger *slog.Logger
}

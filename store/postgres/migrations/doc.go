// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package migrations is the registry of Go schema migrations of PostgreSQL databases used through store/postgres, and their runner.
//
// It is the module github.com/altessa-s/go-atlas/store/postgres/migrations, separate from the go-atlas root module so goose's
// dependency graph (it requires the drivers of every database goose supports) stays out of the root module's consumers.
//
// # Registering
//
// The package owning a table registers its migrations from an init function with [MustRegister] ([Register] returns the error
// instead): an explicit version, a description, and up and down steps that run inside the transaction recording the version.
//
//	func init() {
//		migrations.MustRegister(2026112301, "create items", func(ctx context.Context, tx *sql.Tx) error {
//			_, err := tx.ExecContext(ctx, `CREATE TABLE items (id uuid PRIMARY KEY, name text COLLATE "C" NOT NULL)`)
//			return err
//		}, nil)
//	}
//
// # Running
//
// [Run] applies the pending migrations at startup, holding a PostgreSQL session-level advisory lock so replicas starting together
// migrate once. Supply a direct primary pool and a factory/store/postgres UseMigrator callback. Every call builds its own goose.Provider over a copy of
// the registry with goose's global registry disabled, so no goose global state is read or changed. [Down] reverts the latest applied
// migration and [Status] reports what a database has applied.
//
//	if err := migrations.Run(ctx, db, migrations.WithLogger(logger)); err != nil {
//		return err
//	}
package migrations

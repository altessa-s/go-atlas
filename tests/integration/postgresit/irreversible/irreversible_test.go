// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package irreversible_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/postgres/migrations"
	"github.com/altessa-s/go-atlas/store/postgres/postgrestest"
)

const (
	versionCreate int64 = 2026112401
	versionSeed   int64 = 2026112402
)

func init() {
	migrations.MustRegister(versionCreate, "create table", func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE irr (id int PRIMARY KEY)`)
		return err
	}, nil)
	migrations.MustRegister(versionSeed, "seed", func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO irr VALUES (1)`)
		return err
	}, nil)
}

// Down of a migration registered without a down step fails and keeps the version applied, so the next Run does
// not repeat CREATE TABLE over the table it left behind.
func TestDown_IrreversibleMigrationStaysApplied(t *testing.T) {
	t.Parallel()
	db := postgrestest.New(t, postgrestest.WithDatabasePrefix("pgit_irr_"))

	require.NoError(t, migrations.Run(t.Context(), db))
	err := migrations.Down(t.Context(), db)
	require.ErrorIs(t, err, migrations.ErrIrreversibleMigration)
	require.ErrorContains(t, err, "2026112402")

	status, err := migrations.Status(t.Context(), db)
	require.NoError(t, err)
	for _, s := range status {
		require.True(t, s.Applied, "version %d stays applied", s.Version)
	}
	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM irr`).Scan(&n))
	require.Equal(t, 1, n, "the failed down rolled back")
	require.NoError(t, migrations.Run(t.Context(), db), "nothing is pending, so nothing is repeated")
}

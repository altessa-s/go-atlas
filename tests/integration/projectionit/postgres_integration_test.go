// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package projectionit_test

import (
	"cmp"
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/projection"
	pgproj "github.com/altessa-s/go-atlas/data/projection/translators/postgres"
)

func postgresTable(t *testing.T) (*pgx.Conn, string) {
	t.Helper()

	dsn := envOr("POSTGRES_DSN", "postgres://atlas:atlas@127.0.0.1:15432/atlas?sslmode=disable")
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}
	table := "projection_it_" + uniqueSuffix()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = conn.Exec(ctx, "DROP TABLE IF EXISTS "+table)
		_ = conn.Close(ctx)
	})

	_, err = conn.Exec(t.Context(), `CREATE TABLE `+table+` (
		"id"         BIGINT PRIMARY KEY,
		"name"       TEXT NOT NULL,
		"email"      TEXT NOT NULL,
		"password"   TEXT NOT NULL,
		"created_at" TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	require.NoError(t, err)
	_, err = conn.Exec(t.Context(),
		`INSERT INTO `+table+` ("id","name","email","password") VALUES (1,'ann','ann@example.com','secret')`)
	require.NoError(t, err)
	return conn, table
}

// selectColumns runs SELECT cols and returns the column names the server
// reports.
func selectColumns(t *testing.T, conn *pgx.Conn, table, cols string) []string {
	t.Helper()
	rows, err := conn.Query(t.Context(), "SELECT "+cmp.Or(cols, "*")+" FROM "+table)
	require.NoError(t, err)
	defer rows.Close()

	var names []string
	for _, fd := range rows.FieldDescriptions() {
		names = append(names, fd.Name)
	}
	require.True(t, rows.Next())
	require.NoError(t, rows.Err())
	return names
}

func TestPostgresColumnList(t *testing.T) {
	t.Parallel()
	conn, table := postgresTable(t)

	tr, err := pgproj.NewTranslator(
		projection.WithUntrustedInput(),
		projection.WithAllowedFields("name", "email", "createTime"),
		projection.WithFieldMapping(map[string]string{"createTime": "created_at"}),
		projection.WithRequiredFields("id"),
	)
	require.NoError(t, err)

	cols, err := tr.Translate(projection.Spec{Paths: []string{"createTime", "name"}})
	require.NoError(t, err)
	require.Equal(t, []string{"created_at", "id", "name"}, selectColumns(t, conn, table, cols))

	cols, err = tr.Translate(projection.Spec{})
	require.NoError(t, err)
	require.Equal(t, []string{"created_at", "email", "id", "name"}, selectColumns(t, conn, table, cols),
		"the default is the allow-list, never the password")

	_, err = tr.Translate(projection.Spec{Paths: []string{"password"}})
	require.ErrorIs(t, err, projection.ErrFieldNotAllowed)
}

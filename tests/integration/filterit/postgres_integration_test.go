// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filterit_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	pgtrans "github.com/altessa-s/go-atlas/data/filter/translators/postgres"
	"github.com/altessa-s/go-atlas/tests/integration/filterit"
)

// Quoted identifiers throughout, matching what the translator emits:
// PostgreSQL folds unquoted names to lower case, so an unquoted DDL would
// silently create columns the quoted clause could not find.
const postgresSchema = `CREATE TABLE %s (
	"id"         BIGINT PRIMARY KEY,
	"name"       TEXT NOT NULL,
	"age"        BIGINT NOT NULL,
	"price"      DOUBLE PRECISION NOT NULL,
	"active"     BOOLEAN NOT NULL,
	"status"     TEXT NOT NULL,
	"role"       BIGINT NOT NULL,
	"created_at" TIMESTAMPTZ NOT NULL,
	"deleted_at" TIMESTAMPTZ NULL
)`

const postgresInsert = `INSERT INTO %s
	("id","name","age","price","active","status","role","created_at","deleted_at")
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`

type postgresBackend struct {
	conn  *pgx.Conn
	table string
}

func (*postgresBackend) name() filterit.Backend { return filterit.Postgres }

func (b *postgresBackend) setup(tb testing.TB) {
	tb.Helper()

	dsn := envOr("POSTGRES_DSN", "postgres://atlas:atlas@127.0.0.1:15432/atlas?sslmode=disable")
	conn, err := pgx.Connect(tb.Context(), dsn)
	if err != nil {
		tb.Skipf("postgres not reachable: %v", err)
	}

	b.conn = conn
	b.table = "filter_it_" + uniqueSuffix()
	_, err = conn.Exec(tb.Context(), fmt.Sprintf(postgresSchema, b.table))
	require.NoError(tb, err)

	tb.Cleanup(func() {
		ctx := context.Background()
		_, _ = conn.Exec(ctx, "DROP TABLE IF EXISTS "+b.table)
		_ = conn.Close(ctx)
	})

	b.seed(tb)
}

func (b *postgresBackend) seed(tb testing.TB) {
	tb.Helper()

	for _, row := range filterit.Dataset() {
		_, err := b.conn.Exec(tb.Context(), fmt.Sprintf(postgresInsert, b.table),
			row.ID, row.Name, row.Age, row.Price, row.Active,
			row.Status, row.Role, row.CreatedAt, row.DeletedAt)
		require.NoError(tb, err)
	}
}

func (b *postgresBackend) translator(tb testing.TB, opts ...filter.TranslatorOption) *pgtrans.Translator {
	tb.Helper()

	base := []filter.TranslatorOption{filter.WithFieldMapping(filterit.SQLFieldMapping())}
	tr, err := pgtrans.NewTranslator(append(base, opts...)...)
	require.NoError(tb, err)
	return tr
}

func (b *postgresBackend) search(tb testing.TB, expr string) ([]int64, error) {
	tb.Helper()

	where, args, err := b.translator(tb).Translate(parse(tb, expr))
	if err != nil {
		return nil, err
	}

	rows, err := b.conn.Query(tb.Context(),
		`SELECT "id" FROM `+b.table+" WHERE "+where+` ORDER BY "id"`, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres rejected %q: %w", where, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres rejected %q: %w", where, err)
	}
	return ids, nil
}

func (b *postgresBackend) translate(tb testing.TB, expr string, opts ...filter.TranslatorOption) error {
	tb.Helper()

	_, _, err := b.translator(tb, opts...).Translate(parse(tb, expr))
	return err
}

func TestPostgres(t *testing.T) {
	runCorpus(t, &postgresBackend{})
}

func TestPostgres_Rejections(t *testing.T) {
	runRejections(t, &postgresBackend{})
}

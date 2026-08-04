// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filterit_test

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/go-sql-driver/mysql" // database/sql driver under test
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	mariatrans "github.com/altessa-s/go-atlas/data/filter/translators/mariadb"
	"github.com/altessa-s/go-atlas/tests/integration/filterit"
)

// The schema keeps MariaDB's default collation rather than forcing a
// binary one. Case-insensitive comparison is what a caller gets out of
// the box, so it is what the corpus asserts against.
const mariadbSchema = "CREATE TABLE %s (" +
	"`id` BIGINT PRIMARY KEY," +
	"`name` VARCHAR(64) NOT NULL," +
	"`age` BIGINT NOT NULL," +
	"`price` DOUBLE NOT NULL," +
	"`active` BOOLEAN NOT NULL," +
	"`status` VARCHAR(32) NOT NULL," +
	"`role` BIGINT NOT NULL," +
	"`created_at` DATETIME(6) NOT NULL," +
	"`deleted_at` DATETIME(6) NULL" +
	")"

const mariadbInsert = "INSERT INTO %s " +
	"(`id`,`name`,`age`,`price`,`active`,`status`,`role`,`created_at`,`deleted_at`) " +
	"VALUES (?,?,?,?,?,?,?,?,?)"

type mariadbBackend struct {
	db    *sql.DB
	table string
}

func (*mariadbBackend) name() filterit.Backend { return filterit.MariaDB }

func (b *mariadbBackend) setup(tb testing.TB) {
	tb.Helper()

	dsn := envOr("MARIADB_DSN", "atlas:atlas@tcp(127.0.0.1:13306)/atlas?parseTime=true&loc=UTC")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		tb.Skipf("mariadb not available: %v", err)
	}
	if err = db.PingContext(tb.Context()); err != nil {
		_ = db.Close()
		tb.Skipf("mariadb not reachable: %v", err)
	}

	b.db = db
	b.table = "filter_it_" + uniqueSuffix()
	_, err = db.ExecContext(tb.Context(), fmt.Sprintf(mariadbSchema, b.table))
	require.NoError(tb, err)

	tb.Cleanup(func() {
		_, _ = db.Exec("DROP TABLE IF EXISTS " + b.table)
		_ = db.Close()
	})

	b.seed(tb)
}

func (b *mariadbBackend) seed(tb testing.TB) {
	tb.Helper()

	stmt, err := b.db.PrepareContext(tb.Context(), fmt.Sprintf(mariadbInsert, b.table))
	require.NoError(tb, err)
	defer func() { _ = stmt.Close() }()

	for _, row := range filterit.Dataset() {
		_, err = stmt.ExecContext(tb.Context(),
			row.ID, row.Name, row.Age, row.Price, row.Active,
			row.Status, row.Role, row.CreatedAt, row.DeletedAt)
		require.NoError(tb, err)
	}
}

func (b *mariadbBackend) translator(tb testing.TB, opts ...filter.TranslatorOption) *mariatrans.Translator {
	tb.Helper()

	base := []filter.TranslatorOption{filter.WithFieldMapping(filterit.SQLFieldMapping())}
	tr, err := mariatrans.NewTranslator(append(base, opts...)...)
	require.NoError(tb, err)
	return tr
}

func (b *mariadbBackend) search(tb testing.TB, expr string) ([]int64, error) {
	tb.Helper()

	where, args, err := b.translator(tb).Translate(parse(tb, expr))
	if err != nil {
		return nil, err
	}

	rows, err := b.db.QueryContext(tb.Context(),
		"SELECT `id` FROM "+b.table+" WHERE "+where+" ORDER BY `id`", args...)
	if err != nil {
		return nil, fmt.Errorf("mariadb rejected %q: %w", where, err)
	}
	defer func() { _ = rows.Close() }()

	return collectIDs(rows)
}

func (b *mariadbBackend) translate(tb testing.TB, expr string, opts ...filter.TranslatorOption) error {
	tb.Helper()

	_, _, err := b.translator(tb, opts...).Translate(parse(tb, expr))
	return err
}

func TestMariaDB(t *testing.T) {
	runCorpus(t, &mariadbBackend{})
}

func TestMariaDB_Rejections(t *testing.T) {
	runRejections(t, &mariadbBackend{})
}

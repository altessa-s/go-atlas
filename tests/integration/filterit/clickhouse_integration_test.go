// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filterit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	chtrans "github.com/altessa-s/go-atlas/data/filter/translators/clickhouse"
	"github.com/altessa-s/go-atlas/tests/integration/filterit"
)

const clickhouseSchema = `CREATE TABLE %s (
	id         Int64,
	name       String,
	age        Int64,
	price      Float64,
	active     Bool,
	status     String,
	role       Int64,
	created_at DateTime64(3, 'UTC'),
	deleted_at Nullable(DateTime64(3, 'UTC'))
) ENGINE = MergeTree ORDER BY id`

type clickhouseBackend struct {
	conn  driver.Conn
	table string
}

func (*clickhouseBackend) name() filterit.Backend { return filterit.ClickHouse }

func (b *clickhouseBackend) setup(tb testing.TB) {
	tb.Helper()

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{envOr("CLICKHOUSE_ADDR", "127.0.0.1:19001")},
		Auth: clickhouse.Auth{
			Database: envOr("CLICKHOUSE_DB", "default"),
			Username: envOr("CLICKHOUSE_USER", "atlas"),
			Password: envOr("CLICKHOUSE_PASSWORD", "atlas"),
		},
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		tb.Skipf("clickhouse not available: %v", err)
	}
	if err = conn.Ping(tb.Context()); err != nil {
		_ = conn.Close()
		tb.Skipf("clickhouse not reachable: %v", err)
	}

	b.conn = conn
	b.table = "filter_it_" + uniqueSuffix()
	require.NoError(tb, conn.Exec(tb.Context(), fmt.Sprintf(clickhouseSchema, b.table)))

	tb.Cleanup(func() {
		_ = conn.Exec(context.Background(), "DROP TABLE IF EXISTS "+b.table)
		_ = conn.Close()
	})

	b.seed(tb)
}

func (b *clickhouseBackend) seed(tb testing.TB) {
	tb.Helper()

	batch, err := b.conn.PrepareBatch(tb.Context(), "INSERT INTO "+b.table)
	require.NoError(tb, err)

	for _, row := range filterit.Dataset() {
		require.NoError(tb, batch.Append(
			row.ID, row.Name, row.Age, row.Price, row.Active,
			row.Status, row.Role, row.CreatedAt, row.DeletedAt,
		))
	}
	require.NoError(tb, batch.Send())
}

func (b *clickhouseBackend) translator(tb testing.TB, opts ...filter.TranslatorOption) *chtrans.Translator {
	tb.Helper()

	base := []filter.TranslatorOption{filter.WithFieldMapping(filterit.SQLFieldMapping())}
	tr, err := chtrans.NewTranslator(append(base, opts...)...)
	require.NoError(tb, err)
	return tr
}

func (b *clickhouseBackend) search(tb testing.TB, expr string) ([]int64, error) {
	tb.Helper()

	where, args, err := b.translator(tb).Translate(parse(tb, expr))
	if err != nil {
		return nil, err
	}

	rows, err := b.conn.Query(tb.Context(),
		"SELECT id FROM "+b.table+" WHERE "+where+" ORDER BY id", args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse rejected %q: %w", where, err)
	}
	defer func() { _ = rows.Close() }()

	return collectIDs(rows)
}

func (b *clickhouseBackend) translate(tb testing.TB, expr string, opts ...filter.TranslatorOption) error {
	tb.Helper()

	_, _, err := b.translator(tb, opts...).Translate(parse(tb, expr))
	return err
}

func TestClickHouse(t *testing.T) {
	runCorpus(t, &clickhouseBackend{})
}

func TestClickHouse_Rejections(t *testing.T) {
	runRejections(t, &clickhouseBackend{})
}

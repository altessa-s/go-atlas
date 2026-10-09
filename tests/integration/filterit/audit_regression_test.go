// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filterit_test

import (
	flua "github.com/altessa-s/go-atlas/data/filter/translators/lua"
	"strconv"
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/tests/integration/filterit"
	"github.com/stretchr/testify/require"
)

func TestLiveSQLAudit(t *testing.T) {
	t.Parallel()
	for _, b := range []rowInserter{&postgresBackend{}, &mariadbBackend{}, &clickhouseBackend{}} {
		t.Run(string(b.name()), func(t *testing.T) {
			t.Parallel()
			b.setup(t)
			b.insert(t, []filterit.Row{
				{ID: 201, Name: "active", Status: "active", CreatedAt: filterit.Epoch},
				{ID: 202, Name: "é", Status: "unicode", CreatedAt: filterit.Epoch},
				{ID: 203, Name: "quote' slash\\ newline\n", Status: "escaped", CreatedAt: filterit.Epoch},
			})
			cases := []struct {
				name, expr string
				want       []int64
			}{
				{"field_comparison", `id >= 200 && name == status`, []int64{201}},
				{"field_in_list", `id >= 200 && name in [status]`, []int64{201}},
				{"field_string_argument", `id >= 200 && name.contains(status)`, []int64{201}},
				{"field_prefix", `id >= 200 && name.startsWith(status)`, []int64{201}},
				{"field_suffix", `id >= 200 && name.endsWith(status)`, []int64{201}},
				{"mixed_membership", `id >= 200 && name in [status, "é"] && id < 203`, []int64{201, 202}},
				{"literal_in_dynamic_list", `id >= 200 && "active" in [status, "missing"] && id < 203`, []int64{201}},
				{"unicode_size", `id >= 200 && name.size() == 1`, []int64{202}},
				{"empty_membership", `name in []`, nil},
				{"escaped_literal", `name == "quote' slash\\ newline\n"`, []int64{203}},
			}
			// These subtests share one pgx connection and run serially.
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					got, err := b.search(t, tc.expr)
					t.Logf("expression=%s ids=%v error=%v", tc.expr, got, err)
					require.NoError(t, err)
					require.Equal(t, tc.want, normalize(got))
				})
			}
		})
	}
}

func TestLiveMeiliAudit(t *testing.T) {
	t.Parallel()
	b := &meiliBackend{}
	b.setup(t)
	task, err := b.client.IndexDocuments(t.Context(), b.index, []map[string]any{
		{"id": 201, "name": "active", "status": "active"},
		{"id": 202, "name": "quote\" slash\\ newline\n", "status": "escaped"},
	})
	require.NoError(t, err)
	require.NoError(t, b.client.WaitForTask(t.Context(), task, 0))
	cases := []struct {
		name, expr string
		want       []int64
	}{
		{"field_comparison", `id >= 200 && name == status`, []int64{201}},
		{"field_string_argument", `id >= 200 && name.contains(status)`, []int64{201}},
		{"field_in_list", `id >= 200 && name in [status]`, []int64{201}},
		{"empty_membership", `status in []`, nil},
		{"escaped_literal", `name == "quote\" slash\\ newline\n"`, []int64{202}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := b.search(t, tc.expr)
			t.Logf("expression=%s ids=%v error=%v", tc.expr, got, err)
			if strings.HasPrefix(tc.name, "field_") {
				require.ErrorIs(t, err, filter.ErrUnsupportedOperation)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, normalize(got))
		})
	}
}

func TestLiveRedisAudit(t *testing.T) {
	t.Parallel()
	b := &redisearchBackend{}
	b.setup(t)
	for _, row := range []struct {
		id     int
		status string
	}{{201, `a\|b`}, {202, `a\`}, {203, `b`}, {204, `$param`}} {
		require.NoError(t, b.rdb.HSet(t.Context(), b.prefix+strconv.Itoa(row.id), "id", row.id, "status", row.status).Err())
	}
	t.Run("not_literal_allowlist", func(t *testing.T) {
		t.Parallel()
		tr := b.translator(t, filter.WithUntrustedInput(), filter.WithAllowedFields("name"))
		q, err := tr.Translate(parse(t, `!"@status:{active}"`))
		require.ErrorIs(t, err, filter.ErrInvalidExpression)
		require.Empty(t, q)
	})
	cases := []struct {
		name, expr string
		want       []int64
	}{
		{"backslash_tag", `status == "a\\|b"`, []int64{201}},
		{"dollar_tag", `status == "$param"`, []int64{204}},
		{"negated_empty_membership", `id >= 200 && !(status in [])`, []int64{201, 202, 203, 204}},
		{"empty_tag_membership", `status in []`, nil},
		{"empty_numeric_membership", `age in []`, nil},
		{"empty_text_membership", `name in []`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := b.search(t, tc.expr)
			t.Logf("expression=%s ids=%v error=%v", tc.expr, got, err)
			require.NoError(t, err)
			require.Equal(t, tc.want, normalize(got))
		})
	}
}

func TestLiveLuaRegressions(t *testing.T) {
	t.Parallel()
	b := &redisearchBackend{}
	b.setup(t)
	for _, tc := range []struct{ expr, name string }{
		{`active && other`, ""},
		{`name == "\x001"`, "\x001"},
		{`name.size() == 2`, "é😀"},
		{`tags.size() == 2`, ""},
		{`name == otherName`, "same"},
		{`name in ["missing", otherName]`, "same"},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			t.Parallel()
			tr, err := flua.NewTranslator("d")
			require.NoError(t, err)
			q, err := tr.Translate(parse(t, tc.expr))
			require.NoError(t, err)
			script := `local d = {active = true, other = true, name = ARGV[1], otherName = ARGV[1], tags = {1, 2}}; return ` + q
			got, err := b.rdb.Eval(t.Context(), script, nil, tc.name).Int()
			require.NoError(t, err)
			require.Equal(t, 1, got, q)
		})
	}
}

func TestLiveClickHouseArraySize(t *testing.T) {
	t.Parallel()
	b := &clickhouseBackend{}
	b.setup(t)
	tr := b.translator(t)
	query, args, err := tr.Translate(parse(t, `tags.size() == 2`))
	require.NoError(t, err)
	var count uint64
	require.NoError(t, b.conn.QueryRow(t.Context(), "SELECT count() FROM (SELECT ['é', '😀'] AS tags) WHERE "+query, args...).Scan(&count))
	require.Equal(t, uint64(1), count)
}

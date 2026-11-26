// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgrestest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithDatabase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, dsn, want string
	}{
		{"url", "postgres://u:p@h:5432/atlas?sslmode=disable", "postgres://u:p@h:5432/it_x?sslmode=disable"},
		{"mixed case and whitespace", "  PoSTgreSQL://h/admin?database=other  ", "postgresql://h/it_x"},
		{"postgresql url without path", "postgresql://u@h", "postgresql://u@h/it_x"},
		{"url with escaped password", "postgres://u:p%40ss@h/atlas", "postgres://u:p%40ss@h/it_x"},
		// Pgx applies a dbname or database query parameter after the path, so both must go.
		{"url with dbname and database parameters", "postgres://h/admin?dbname=shared&sslmode=disable&database=other",
			"postgres://h/it_x?sslmode=disable"},
		{"escaped trailing password space", `host=h password=a\ `, `host=h password=a\  dbname=it_x`},
		{"keyword value", "host=h user=u dbname=atlas", "host=h user=u dbname=atlas dbname=it_x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, withDatabase(tc.dsn, "it_x"))
		})
	}
}

func TestQuoteIdent(t *testing.T) {
	t.Parallel()

	require.Equal(t, `"it_x"`, quoteIdent("it_x"))
	require.Equal(t, `"a""b"`, quoteIdent(`a"b`))
}

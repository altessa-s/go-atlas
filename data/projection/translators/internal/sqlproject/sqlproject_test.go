// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlproject_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/projection"
	"github.com/altessa-s/go-atlas/data/projection/translators/internal/sqlproject"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

func TestNewContext(t *testing.T) {
	t.Parallel()

	_, err := sqlproject.NewContext(sqldialect.Postgres, projection.WithDeniedFields("password"))
	require.ErrorIs(t, err, projection.ErrDefaultFieldsRequired, "SQL cannot express an exclusion")

	_, err = sqlproject.NewContext(sqldialect.Postgres, projection.WithDeniedFields("password"), projection.WithDefaultFields("name"))
	require.NoError(t, err)

	_, err = sqlproject.NewContext(sqldialect.Postgres, projection.WithUntrustedInput())
	require.ErrorIs(t, err, projection.ErrAllowlistRequired)

	_, err = sqlproject.NewContext(sqldialect.Postgres,
		projection.WithAllowedFields("raw"),
		projection.WithFieldMapping(map[string]string{"raw": "lower(name)"}),
	)
	require.ErrorIs(t, err, projection.ErrInvalidFieldPath, "a default that is not a column fails at construction")

	ctx, err := sqlproject.NewContext(sqldialect.MySQL, projection.WithAllowedFields("name", "author.*"))
	require.NoError(t, err)
	got, err := sqlproject.Translate(ctx, sqldialect.MySQL, projection.Spec{})
	require.NoError(t, err)
	require.Equal(t, "`author`, `name`", got, "allow-list roots are the default")
}

func TestTranslate(t *testing.T) {
	t.Parallel()

	ctx, err := sqlproject.NewContext(sqldialect.Postgres,
		projection.WithAllowedFields("name", "createTime", "author.*", "raw"),
		projection.WithFieldMapping(map[string]string{
			"createTime":  "created_at",
			"author.name": "authors.name",
			"author.bio":  "a.b.c",
			"raw":         "lower(name)",
		}),
		projection.WithRequiredFields("id"),
		projection.WithDefaultFields("name"),
	)
	require.NoError(t, err)

	tests := []struct {
		name    string
		paths   []string
		style   sqldialect.Style
		want    string
		wantErr error
	}{
		{name: "default fields", style: sqldialect.Postgres, want: `"id", "name"`},
		{name: "postgres quoting", paths: []string{"name", "createTime"}, style: sqldialect.Postgres, want: `"created_at", "id", "name"`},
		{name: "mysql quoting", paths: []string{"name"}, style: sqldialect.MySQL, want: "`id`, `name`"},
		{name: "table qualified", paths: []string{"author.name"}, style: sqldialect.Postgres, want: `"authors"."name", "id"`},
		{name: "too deep for SQL", paths: []string{"author.bio"}, style: sqldialect.Postgres, wantErr: projection.ErrInvalidFieldPath},
		{name: "expression rejected", paths: []string{"raw"}, style: sqldialect.Postgres, wantErr: projection.ErrInvalidFieldPath},
		{name: "not allowed", paths: []string{"secret"}, style: sqldialect.Postgres, wantErr: projection.ErrFieldNotAllowed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := sqlproject.Translate(ctx, tc.style, projection.Spec{Paths: tc.paths})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTranslateAllColumns(t *testing.T) {
	t.Parallel()

	ctx, err := sqlproject.NewContext(sqldialect.Postgres)
	require.NoError(t, err)
	got, err := sqlproject.Translate(ctx, sqldialect.Postgres, projection.Spec{})
	require.NoError(t, err)
	require.Empty(t, got, "every column renders empty so the caller writes *")
}

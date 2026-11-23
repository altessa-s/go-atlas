// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"database/sql/driver"
	"embed"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestDiscoverMigrations(t *testing.T) {
	t.Parallel()

	source := fstest.MapFS{
		"000010_indexes.up.sql": {Data: []byte("SELECT 10")},
		"000001_initial.up.sql": {Data: []byte("SELECT 1")},
		"000002_extra.up.sql":   {Data: []byte("SELECT 2")},
	}

	got, err := discoverMigrations(source)
	if err != nil {
		t.Fatal(err)
	}

	versions := make([]int64, len(got))
	for i, version := range got {
		versions[i] = version.version
		if _, err = source.ReadFile(version.name); err != nil {
			t.Fatal(err)
		}
	}

	if !reflect.DeepEqual(versions, []int64{1, 2, 10}) {
		t.Fatalf("discovered versions=%v", versions)
	}
}

func TestDiscoverMigrationsRejectsAmbiguousFiles(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		files fstest.MapFS
	}{
		{
			"duplicate version",
			fstest.MapFS{"1_first.up.sql": {}, "000001_other.up.sql": {}},
		},
		{"invalid version", fstest.MapFS{"bad_initial.up.sql": {}}},
		{"missing name", fstest.MapFS{"000001_.up.sql": {}}},
		{"unhandled direction", fstest.MapFS{"000001_initial.down.sql": {}}},
		{"directory", fstest.MapFS{"000001_initial.up.sql/schema.sql": {}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := discoverMigrations(test.files); err == nil {
				t.Fatal("ambiguous migration set accepted")
			}
		})
	}
}

func TestMigrateLedger(t *testing.T) {
	source := fstest.MapFS{"1_first.up.sql": {Data: []byte("SELECT 11")}, "2_second.up.sql": {Data: []byte("SELECT 22")}}
	for _, fail := range []bool{false, true} {
		cause := errors.New("migration failed")
		db, fake := testhelpers.NewFakeSQL(t, func(query string, args []any) testhelpers.FakeSQLReply {
			switch {
			case strings.Contains(query, "SELECT EXISTS"):
				return testhelpers.FakeSQLReply{Columns: []string{"exists"}, Rows: [][]driver.Value{{args[0] == int64(1)}}}
			case query == "SELECT 22" && fail:
				return testhelpers.FakeSQLReply{Err: cause}
			}
			return testhelpers.FakeSQLReply{}
		})
		err := Migrate(t.Context(), db, source, "catalog")
		if fail {
			require.ErrorIs(t, err, cause)
			require.Equal(t, 1, fake.Rollbacks())
		} else {
			require.NoError(t, err)
			require.Equal(t, 1, fake.Commits())
		}
		var statements []string
		for _, call := range fake.Calls() {
			require.True(t, call.InTx)
			statements = append(statements, call.Query)
		}
		require.NotContains(t, statements, "SELECT 11")
		require.Contains(t, statements, "SELECT 22")
		require.Contains(t, statements[0], "pg_advisory_xact_lock")
		require.Error(t, Migrate(t.Context(), db, nil, "catalog"))
		require.Error(t, Migrate(t.Context(), db, (*embed.FS)(nil), "catalog"))
		require.Error(t, Migrate(t.Context(), db, source, strings.Repeat("x", 64)))
		require.Error(t, WithTransaction(t.Context(), db, func(ctx context.Context) error { return Migrate(ctx, db, source, "catalog") }))
	}
}

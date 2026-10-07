// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auditit_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/sqldb"
	"github.com/altessa-s/go-atlas/data/audit/storages/storagetest"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// sqlTarget is one database/sql server of the docker stack.
type sqlTarget struct {
	name, driver, envKey, dsn string
	dialect                   sqldb.Dialect
}

func sqlTargets() []sqlTarget {
	return []sqlTarget{
		{"postgres", "pgx", "POSTGRES_DSN", "postgres://atlas:atlas@127.0.0.1:15432/atlas?sslmode=disable", sqldb.DialectPostgres},
		{"mariadb", "mysql", "MARIADB_DSN", "atlas:atlas@tcp(127.0.0.1:13306)/atlas", sqldb.DialectMySQL},
		{"mysql", "mysql", "MYSQL_DSN", "atlas:atlas@tcp(127.0.0.1:13307)/atlas", sqldb.DialectMySQL},
	}
}

// storage opens the server and returns a storage over a throwaway table,
// created with EnsureSchema twice (idempotence) and dropped on cleanup. It
// skips when the server is unreachable.
func (s sqlTarget) storage(t *testing.T, opts ...sqldb.Option) *sqldb.Storage {
	t.Helper()
	st, _, _ := s.open(t, opts...)
	return st
}

// open is storage that also returns the database handle and table name.
func (s sqlTarget) open(t *testing.T, opts ...sqldb.Option) (*sqldb.Storage, *sql.DB, string) {
	t.Helper()
	dsn := s.dsn
	if v := os.Getenv(s.envKey); v != "" {
		dsn = v
	}
	db, err := sql.Open(s.driver, dsn)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		// The DSN is not printed: an override may carry a password.
		t.Skipf("%s unreachable (%v) — start it with: make integration-up", s.name, err)
	}
	table := "audit_" + strings.ToLower(rand.Text()[:12])
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
		_ = db.Close()
	})
	st, err := sqldb.New(db, s.dialect, append(opts, sqldb.WithTableName(table))...)
	require.NoError(t, err)
	require.NoError(t, st.EnsureSchema(t.Context()))
	require.NoError(t, st.EnsureSchema(t.Context()))
	return st, db, table
}

// TestConformance runs the audit storage conformance suite on every SQL
// server. The storage accepts replayed events, so no rejection is expected.
func TestConformance(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			storagetest.Run(t, func(t *testing.T) audit.Storage { return target.storage(t) }, nil)
		})
	}
}

func event(id string, at time.Time) *audit.Event {
	return &audit.Event{ID: id, Type: "test", Action: "read", Timestamp: at, Actor: audit.Actor{ID: "u1", Type: "user"},
		Result: audit.Result{Status: "success"}, Metadata: map[string]any{"note": "ü"}}
}

func ids(t *testing.T, st audit.Storage) []string {
	t.Helper()
	var out []string
	for e, err := range st.Query(t.Context(), &audit.Query{SortOrder: audit.SortOrderAsc}) {
		require.NoError(t, err)
		out = append(out, e.ID)
	}
	return out
}

// TestBatchAtomicAndIdempotent stores a batch split into several INSERTs,
// replays it, and then sends a batch whose last INSERT fails: the replay adds
// nothing and the failing batch leaves no row behind.
func TestBatchAtomicAndIdempotent(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 3, 4, 5, 6, 7, 891_234_567, time.UTC)
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			st, db, table := target.open(t, sqldb.WithMaxBatchRows(2))

			batch := make([]*audit.Event, 5)
			want := make([]string, len(batch))
			for i := range batch {
				batch[i] = event(fmt.Sprintf("e%d", i), base.Add(time.Duration(i)*time.Second))
				want[i] = batch[i].ID
			}
			require.NoError(t, st.StoreBatch(t.Context(), batch))
			require.NoError(t, st.StoreBatch(t.Context(), batch), "a replayed batch is accepted")
			require.Equal(t, want, ids(t, st))

			var got *audit.Event
			for e, err := range st.Query(t.Context(), &audit.Query{Limit: 1, SortOrder: audit.SortOrderAsc}) {
				require.NoError(t, err)
				got = e
			}
			require.True(t, base.Equal(got.Timestamp), "the timestamp keeps full precision")
			require.Equal(t, "ü", got.Metadata["note"])

			// The third INSERT carries a row the server rejects, after the
			// first two INSERTs ran.
			_, err := db.ExecContext(t.Context(), "ALTER TABLE "+table+" ADD CONSTRAINT "+table+"_no_boom CHECK (actor_id <> 'boom')")
			require.NoError(t, err)
			rejected := event("n4", base)
			rejected.Actor.ID = "boom"
			bad := []*audit.Event{event("n0", base), event("n1", base), event("n2", base), event("n3", base), rejected}
			require.Error(t, st.StoreBatch(t.Context(), bad))
			require.Equal(t, want, ids(t, st), "a failed batch leaves no row behind")
		})
	}
}

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

// Each reply consumes one statement on the single physical session. An extra checkout or
// any SQL before verification fails the script, even if a subsequent unlock succeeds.
type reply struct {
	query  string
	fields []uint32
	values []string
	status byte
}

func scriptedDB(tb testing.TB, replies []reply) *sql.DB {
	tb.Helper()
	cfg, err := pgx.ParseConfig("postgres://test@127.0.0.1/test?sslmode=disable")
	require.NoError(tb, err)
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	client, server := net.Pipe()
	done := make(chan error, 1)
	go func() { defer server.Close(); done <- serveReplies(pgproto3.NewBackend(server, server), replies) }()
	cfg.DialFunc = func(context.Context, string, string) (net.Conn, error) { return client, nil }
	db := stdlib.OpenDB(*cfg, stdlib.OptionShouldPing(func(context.Context, stdlib.ShouldPingParams) bool { return false }))
	db.SetMaxOpenConns(1)
	tb.Cleanup(func() { _ = db.Close(); _ = client.Close(); require.NoError(tb, <-done) })
	return db
}

func serveReplies(b *pgproto3.Backend, replies []reply) error {
	if _, err := b.ReceiveStartupMessage(); err != nil {
		return err
	}
	b.Send(&pgproto3.AuthenticationOk{})
	b.Send(&pgproto3.ParameterStatus{Name: "standard_conforming_strings", Value: "on"})
	b.Send(&pgproto3.ParameterStatus{Name: "client_encoding", Value: "UTF8"})
	b.Send(&pgproto3.BackendKeyData{ProcessID: 17, SecretKey: []byte{0, 0, 0, 1}})
	b.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := b.Flush(); err != nil {
		return err
	}
	for _, r := range replies {
		msg, err := b.Receive()
		if err != nil {
			return err
		}
		q, ok := msg.(*pgproto3.Query)
		if !ok || !strings.Contains(strings.ToLower(q.String), r.query) {
			return fmt.Errorf("expected %q, received %v", r.query, msg)
		}
		if len(r.fields) > 0 {
			fields := make([]pgproto3.FieldDescription, len(r.fields))
			values := make([][]byte, len(r.fields))
			for i, oid := range r.fields {
				fields[i].DataTypeOID = oid
				values[i] = []byte(r.values[i])
			}
			b.Send(&pgproto3.RowDescription{Fields: fields})
			b.Send(&pgproto3.DataRow{Values: values})
		}
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("OK")})
		b.Send(&pgproto3.ReadyForQuery{TxStatus: r.status})
		if err := b.Flush(); err != nil {
			return err
		}
	}
	msg, err := b.Receive()
	if err != nil {
		return err
	}
	if _, ok := msg.(*pgproto3.Terminate); !ok {
		return fmt.Errorf("unexpected statement after script: %v", msg)
	}
	return nil
}

func TestSessionProtocol(t *testing.T) { //nolint:paralleltest // swaps the process-wide registry
	isolateRegistry(t)
	MustRegister(17, "test", func(ctx context.Context, _ *sql.Tx) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < time.Second {
			return errors.New("work inherited the lock budget")
		}
		return fmt.Errorf("password=secret: %w", ErrInvalidMigration)
	}, nil)
	identity := reply{"pg_backend_pid()", []uint32{pgtype.Int4OID, pgtype.TextOID}, []string{"17", "test"}, 'I'}
	t.Run("pooler", func(t *testing.T) {
		t.Parallel()
		mismatch := identity
		mismatch.values = []string{"42", "test"}
		db := scriptedDB(t, []reply{mismatch})
		require.ErrorIs(t, Run(t.Context(), db), ErrDirectConnectionRequired)
	})
	t.Run("nothing applied", func(t *testing.T) {
		t.Parallel()
		db := scriptedDB(t, []reply{
			identity, {query: "pg_advisory_lock(", status: 'I'},
			{"from pg_tables", []uint32{pgtype.BoolOID}, []string{"t"}, 'I'},
			{"select version_id, is_applied", []uint32{pgtype.Int8OID, pgtype.BoolOID}, []string{"0", "t"}, 'I'},
			{"pg_advisory_unlock(", []uint32{pgtype.BoolOID}, []string{"t"}, 'I'},
		})
		require.NoError(t, Down(t.Context(), db))
	})
	t.Run("locked rollback", func(t *testing.T) {
		t.Parallel()
		ledger := reply{"select version_id, is_applied", []uint32{pgtype.Int8OID, pgtype.BoolOID}, []string{"0", "t"}, 'I'}
		db := scriptedDB(t, []reply{
			identity, {query: "pg_advisory_lock(", status: 'I'},
			{"from pg_tables", []uint32{pgtype.BoolOID}, []string{"t"}, 'I'},
			ledger, ledger, {query: "begin", status: 'T'}, {query: "rollback", status: 'I'},
			{"pg_advisory_unlock(", []uint32{pgtype.BoolOID}, []string{"t"}, 'I'},
		})
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		err := Run(ctx, db, WithLockTimeout(500*time.Millisecond))
		require.ErrorIs(t, err, ErrInvalidMigration)
		require.ErrorContains(t, err, "version 17")
		require.NotContains(t, err.Error(), "secret")
	})
}

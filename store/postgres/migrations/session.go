// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package migrations

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/altessa-s/go-atlas/store/postgres"
)

func directConn(ctx context.Context, db *sql.DB) (*sql.Conn, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, errors.Join(err, ctx.Err())
	}
	err = conn.Raw(func(raw any) error {
		c, ok := raw.(interface{ Conn() *pgx.Conn })
		if !ok {
			return ErrDirectConnectionRequired
		}
		return postgres.CheckDirect(ctx, c.Conn(), "")
	})
	if err != nil {
		_ = conn.Close()
		return nil, errors.Join(err, ctx.Err())
	}
	return conn, nil
}

// Goose takes a *sql.DB and checks out connections several times during Up. Lend it
// only the locked physical session, including for the initial ledger check. Reconnecting
// after a lost session would let work proceed without the lock.
type pgDriverConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.QueryerContext
	driver.ExecerContext
	driver.ConnPrepareContext
	driver.NamedValueChecker
}

type borrowedConn struct{ pgDriverConn }

func (borrowedConn) Close() error { return nil }

type singleSession struct{ conn driver.Conn }

func (s *singleSession) Connect(context.Context) (driver.Conn, error) {
	if s.conn == nil {
		return nil, sql.ErrConnDone
	}
	conn := s.conn
	s.conn = nil
	return conn, nil
}

func (s *singleSession) Driver() driver.Driver { return s }

// A DSN cannot open a replacement for the borrowed, locked session.
func (*singleSession) Open(string) (driver.Conn, error) { return nil, sql.ErrConnDone }

func withLock(ctx, acquire context.Context, conn *sql.Conn, o *options, work func(*sql.DB) error) (err error) {
	// On an ambiguous acquisition or release, discard the physical session. Returning it
	// to the caller's pool could transfer a still-held, reentrant advisory lock.
	discard := true
	defer func() {
		if discard {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	if _, err = conn.ExecContext(acquire, "SELECT pg_advisory_lock($1)", o.lockID); err != nil {
		return errors.Join(err, acquire.Err())
	}
	defer func() {
		release, cancel := context.WithTimeout(context.WithoutCancel(ctx), o.lockTimeout)
		defer cancel()
		var unlocked bool
		releaseErr := conn.QueryRowContext(release, "SELECT pg_advisory_unlock($1)", o.lockID).Scan(&unlocked)
		if releaseErr == nil && !unlocked {
			releaseErr = errors.New("migration advisory lock was lost")
		}
		discard = releaseErr != nil
		err = errors.Join(err, releaseErr)
	}()
	return conn.Raw(func(raw any) error {
		physical, ok := raw.(pgDriverConn)
		if !ok {
			return ErrDirectConnectionRequired
		}
		db := sql.OpenDB(&singleSession{conn: borrowedConn{physical}})
		db.SetMaxOpenConns(1)
		defer func() { _ = db.Close() }()
		return work(db)
	})
}

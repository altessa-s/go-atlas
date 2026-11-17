// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

// pgCloseTimeout bounds closing a listening connection whose context ended.
const pgCloseTimeout = 5 * time.Second

// PGListener implements outboxsql.Listener over pgx: each subscription opens a
// dedicated connection, runs LISTEN and loops on WaitForNotification. A burst
// of notifications collapses into one pending signal. The subscription ends —
// its channel closes — when ctx ends or the connection fails; resubscribing is
// the store's job.
type PGListener struct {
	// DSN is the PostgreSQL connection string.
	DSN string
}

var _ outboxsql.Listener = PGListener{}

// Listen implements outboxsql.Listener.
func (l PGListener) Listen(ctx context.Context, channel string) (<-chan struct{}, error) {
	conn, err := pgx.Connect(ctx, l.DSN)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		closeConn(ctx, conn)
		return nil, err
	}

	signals := make(chan struct{}, 1)
	go func() {
		defer close(signals)
		defer closeConn(ctx, conn)
		for {
			if _, err := conn.WaitForNotification(ctx); err != nil {
				return
			}
			select {
			case signals <- struct{}{}:
			default: // A signal is already pending; it covers this one too.
			}
		}
	}()
	return signals, nil
}

func closeConn(ctx context.Context, conn *pgx.Conn) {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pgCloseTimeout)
	defer cancel()
	_ = conn.Close(closeCtx)
}

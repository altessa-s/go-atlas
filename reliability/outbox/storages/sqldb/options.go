// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

//go:generate go tool optgen generate

import (
	"context"
	"database/sql"
	"log/slog"
)

// DefaultTableName is the default table name for outbox events; it matches the
// MongoDB store's default collection name.
const DefaultTableName = "events_outbox"

type options struct {
	// WithKeyOrder restricts claims to the earliest undelivered event for each ordering group, in insertion order.
	keyOrder bool
	// WithAsynchronousCommit disables synchronous WAL flush for PostgreSQL dispatch bookkeeping. MySQL ignores it.
	asynchronousCommit bool
	// WithTxResolver finds the application transaction for a call.
	txResolver func(context.Context) (*sql.Tx, bool) `optgen:"notnil"`
	tableName  string                                `optval:"nonempty" optgen:"default=DefaultTableName"`
	// WithNotify makes every save signal the notification channel with
	// pg_notify in the transaction that inserts the events, so a [Listener]
	// learns of them the moment they commit. The channel is derived from the
	// table name unless [WithNotifyChannel] names it. PostgreSQL only: [New]
	// rejects it on MySQL with [ErrNotifyUnsupported].
	notify bool
	// WithNotifyChannel enables notifications like [WithNotify] on the given
	// channel instead of the one derived from the table name. The channel must
	// be a plain identifier of at most 63 characters ([ErrInvalidNotifyChannel]).
	// Names are case-sensitive: pg_notify does not fold case, so psql users
	// must quote mixed-case names in LISTEN. The derived channel is lower case.
	notifyChannel string `optgen:"enables=notify"`
	// WithListener sets the [Listener] [Store.Watch] subscribes through. Without
	// one — nil, or a typed nil — the store reports [outbox.ErrWatchUnsupported].
	// PostgreSQL only: [New] rejects it on MySQL with [ErrNotifyUnsupported].
	listener Listener
	// WithLogger sets the logger [Store.Watch] reports failed subscriptions on.
	// Nothing is logged by default.
	logger *slog.Logger `optgen:"default=slog.New(slog.DiscardHandler),notnil"`
}

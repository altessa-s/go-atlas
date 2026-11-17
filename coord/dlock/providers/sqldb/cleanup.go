// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/panics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// PurgeExpired runs one sweep of [WithExpiredCleanup] now: it deletes, in
// batches, every row whose lease expired or was released more than the grace
// ago by the database clock, and returns how many it deleted. Each batch is
// bounded by the operations timeout. It is safe to run concurrently with lock
// operations and with other sweeps, on any replica. Without
// [WithExpiredCleanup] it deletes nothing.
//
// With a positive purge interval, the locker sweeps from its first Lock
// until Close; call PurgeExpired to sweep at other times, for example
// from a scheduler.
func (l *Locker) PurgeExpired(ctx context.Context) (int64, error) {
	if !l.s.cleanup {
		return 0, nil
	}
	var total int64
	for {
		deleted, selected, err := l.deleteExpiredBatch(ctx)
		total += deleted
		if err != nil {
			return total, coreerrs.WrapOperation(err, "delete expired SQL locks")
		}
		if selected < int64(l.s.purgeBatchSize) {
			return total, nil
		}
	}
}

// deleteExpiredBatch deletes one batch of expired rows. selected is the size
// of the batch the statement considered, which only a full batch reaches.
func (l *Locker) deleteExpiredBatch(ctx context.Context) (deleted, selected int64, err error) {
	ctx, cancel := context.WithTimeout(ctx, l.opTimeout)
	defer cancel()
	s := l.s
	if s.dialect.name == DialectPostgres {
		res, execErr := s.db.ExecContext(ctx, s.stmts.deleteExpired, s.graceUs)
		if execErr != nil {
			return 0, 0, execErr
		}
		n, execErr := res.RowsAffected()
		return n, n, execErr
	}

	keys, err := s.expiredKeys(ctx)
	if err != nil || len(keys) == 0 {
		return 0, 0, err
	}
	selected = int64(len(keys))
	query := fmt.Sprintf(s.stmts.deleteExpired, strings.Repeat(", ?", len(keys))[2:])
	keys = append(keys, s.graceUs)
	res, err := s.db.ExecContext(ctx, query, keys...)
	if err != nil {
		return 0, selected, err
	}
	deleted, err = res.RowsAffected()
	return deleted, selected, err
}

// expiredKeys reads, without locking, the MySQL keys of up to one batch of
// expired rows.
func (s *store) expiredKeys(ctx context.Context) ([]any, error) {
	rows, err := s.db.QueryContext(ctx, s.stmts.expiredKeys, s.graceUs)
	if err != nil {
		return nil, err
	}
	var keys []any
	for rows.Next() {
		var key []byte
		if err := rows.Scan(&key); err != nil {
			_ = rows.Close()
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, errors.Join(rows.Err(), rows.Close())
}

// startSweep starts the background sweep of [WithExpiredCleanup] once, unless
// the locker was closed. The sweep keeps ctx's values but not its
// cancellation: it runs until Close.
func (l *Locker) startSweep(ctx context.Context) {
	if !l.s.cleanup || l.sweepEvery <= 0 {
		return
	}
	l.sweepMu.Lock()
	defer l.sweepMu.Unlock()
	if l.sweepStarted || l.sweepStopped {
		return
	}
	l.sweepStarted = true
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	l.cancelSweep, l.sweepDone = cancel, make(chan struct{})
	go l.sweepLoop(ctx, l.sweepEvery)
}

// stopSweep stops the background sweep for good and returns a channel closed
// once a sweep in progress returned, or nil when no sweep was started.
func (l *Locker) stopSweep() <-chan struct{} {
	l.sweepMu.Lock()
	defer l.sweepMu.Unlock()
	l.sweepStopped = true
	if !l.sweepStarted {
		return nil
	}
	l.cancelSweep()
	return l.sweepDone
}

// awaitSweep waits, within ctx, for done from [Locker.stopSweep].
func (l *Locker) awaitSweep(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	// A sweep that already exited satisfies even an ended ctx.
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return coreerrs.Wrap(ctx.Err(), "wait for the expired lock cleanup to stop")
	}
}

// sweepLoop runs [Locker.PurgeExpired] every period until ctx ends.
func (l *Locker) sweepLoop(ctx context.Context, period time.Duration) {
	defer close(l.sweepDone)
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.runPurge(ctx)
		}
	}
}

// runPurge recovers each pass so future sweeps still run after a panic.
func (l *Locker) runPurge(ctx context.Context) {
	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(ctx context.Context, _ any) {
		l.logger.ErrorContext(ctx, "expired lock purge panicked")
	})
	n, err := l.PurgeExpired(ctx)
	if err != nil && ctx.Err() == nil {
		l.logger.WarnContext(ctx, "failed to purge expired locks", slog.Any("error", err), slog.Int64("deleted", n))
	}
}

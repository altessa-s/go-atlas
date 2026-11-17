// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/altessa-s/go-atlas/internal/purgeloop"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// PurgeExpired runs one sweep of [WithExpiredCleanup] now: it deletes, in
// batches, every row whose lease expired or was released more than the grace
// ago by the database clock, and returns how many it deleted. Each statement is
// bounded by the operations timeout (WithOperationsTimeout). It is safe to run
// concurrently with lock operations and with other sweeps, on any replica. Without
// [WithExpiredCleanup] it deletes nothing.
//
// With a positive purge interval, the locker sweeps from its first Lock
// until Close; call PurgeExpired to sweep at other times, for example
// from a scheduler.
func (l *Locker) PurgeExpired(ctx context.Context) (int64, error) {
	if !l.s.cleanup {
		return 0, nil
	}
	total, err := purgeloop.Drain(ctx, l.s.purgeBatchSize, l.opTimeout, l.deleteExpiredBatch)
	return total, coreerrs.WrapOperation(err, "delete expired SQL locks")
}

// deleteExpiredBatch deletes one batch of expired rows. selected is the size
// of the batch the statement considered, which only a full batch reaches.
func (l *Locker) deleteExpiredBatch(ctx context.Context) (deleted, selected int64, err error) {
	s := l.s
	if s.dialect.name == DialectPostgres {
		res, execErr := s.db.ExecContext(ctx, s.stmts.deleteExpired, s.graceUs, s.purgeBatchSize)
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
	rows, err := s.db.QueryContext(ctx, s.stmts.expiredKeys, s.graceUs, s.purgeBatchSize)
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

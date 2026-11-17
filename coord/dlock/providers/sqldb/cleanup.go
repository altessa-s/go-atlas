// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"

	"github.com/altessa-s/go-atlas/internal/purgeloop"
	"github.com/altessa-s/go-atlas/internal/sqldialect"

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

	keys, err := sqldialect.QueryKeys(ctx, s.db, s.purgeBatchSize, s.stmts.expiredKeys, s.graceUs, s.purgeBatchSize)
	if err != nil || len(keys) == 0 {
		return 0, 0, err
	}
	selected = int64(len(keys))
	query := sqldialect.InList(s.stmts.deleteExpired, len(keys))
	keys = append(keys, s.graceUs)
	res, err := s.db.ExecContext(ctx, query, keys...)
	if err != nil {
		return 0, selected, err
	}
	deleted, err = res.RowsAffected()
	return deleted, selected, err
}

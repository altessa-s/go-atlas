// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// Close permanently stops background purging and waits for it within ctx.
// Explicit PurgeExpired calls remain available. It does not close the database.
func (s *Storage) Close(ctx context.Context) error { return s.purge.Close(ctx) }

// deleteExpiredBatch uses the caller's transaction for explicit purges. MySQL
// selects keys without locking, then rechecks expiry while deleting by key so
// a key renewed between the statements survives.
func (s *Storage) deleteExpiredBatch(ctx context.Context) (int64, int64, error) {
	ex := s.db
	if s.dialect.name == DialectPostgres {
		n, err := execCount(ctx, ex, s.stmts.cleanup, s.purgeBatchSize)
		return n, n, err
	}
	keys, err := sqldialect.QueryKeys(ctx, ex, s.purgeBatchSize, s.stmts.expiredKeys, s.purgeBatchSize)
	if err != nil || len(keys) == 0 {
		return 0, 0, err
	}
	n, err := execCount(ctx, ex, sqldialect.InList(s.stmts.cleanup, len(keys)), keys...)
	return n, int64(len(keys)), err
}

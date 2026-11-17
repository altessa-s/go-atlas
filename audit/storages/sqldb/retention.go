// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"

	"github.com/altessa-s/go-atlas/internal/purgeloop"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// PurgeExpired deletes events whose stored timestamp in milliseconds is older
// than the application clock minus TTL. A single cutoff is used for the entire
// pass. Deletes use the timestamp index in batches of at most WithPurgeBatchSize
// rows, each committed independently, until a batch is not full or ctx ends.
// Each statement is bounded by purgeloop.DefaultStepTimeout (30 seconds).
// It returns the total deleted, including completed batches before an error.
// With TTL zero it returns 0, nil without accessing the database.
//
// Callers may also run this method through their own scheduler; Close stops
// only the background loop, and does not prevent explicit retention calls.
// The background loop starts with the first Store or StoreBatch call.
func (s *Storage) PurgeExpired(ctx context.Context) (int64, error) {
	if s.opts.ttl == 0 {
		return 0, nil
	}
	cutoff := s.opts.clock.Now().Add(-s.opts.ttl).UnixMilli()
	query := "DELETE FROM " + s.table + " WHERE ts_ms < ? ORDER BY ts_ms, id LIMIT ?"
	if s.dialect.name == DialectPostgres {
		query = "DELETE FROM " + s.table + " WHERE id IN (SELECT id FROM " + s.table +
			" WHERE ts_ms < ? ORDER BY ts_ms, id LIMIT ? FOR UPDATE SKIP LOCKED)"
		query = s.dialect.Bind(query, 1)
	}
	total, err := purgeloop.Drain(ctx, s.opts.purgeBatchSize, purgeloop.DefaultStepTimeout,
		func(ctx context.Context) (int64, int64, error) {
			result, err := s.db.ExecContext(ctx, query, cutoff, s.opts.purgeBatchSize)
			if err != nil {
				return 0, 0, err
			}
			n, err := result.RowsAffected()
			return n, n, coreerrs.WrapOperation(err, "count purged audit events")
		})
	return total, coreerrs.WrapOperation(err, "purge expired audit events")
}

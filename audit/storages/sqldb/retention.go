// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/panics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// PurgeExpired deletes events whose stored timestamp in milliseconds is older
// than the application clock minus TTL. A single cutoff is used for the entire
// pass. Deletes use the timestamp index in batches of at most WithPurgeBatchSize
// rows, each committed independently, until a batch is not full or ctx ends.
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
			" WHERE ts_ms < ? ORDER BY ts_ms, id LIMIT ?)"
		query = s.dialect.Bind(query, 1)
	}
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, coreerrs.WrapOperation(err, "purge expired audit events")
		}
		result, err := s.db.ExecContext(ctx, query, cutoff, s.opts.purgeBatchSize)
		if err != nil {
			return total, coreerrs.WrapOperation(err, "purge expired audit events")
		}
		n, err := result.RowsAffected()
		if err != nil {
			return total, coreerrs.WrapOperation(err, "count purged audit events")
		}
		total += n
		if n < int64(s.opts.purgeBatchSize) {
			return total, nil
		}
	}
}

// startPurge starts the background retention loop once when TTL > 0, unless
// the storage was closed. The loop keeps ctx's values but not its
// cancellation: it runs until Close.
func (s *Storage) startPurge(ctx context.Context) {
	if s.opts.ttl == 0 || s.opts.purgeInterval <= 0 {
		return
	}
	s.purgeMu.Lock()
	defer s.purgeMu.Unlock()
	if s.purgeStarted || s.purgeStopped {
		return
	}
	s.purgeStarted = true
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.purgeCancel, s.purgeDone = cancel, make(chan struct{})
	go s.purgeLoop(ctx)
}

// stopPurge stops the background retention loop for good and returns a
// channel closed once it exited, or nil when it never started.
func (s *Storage) stopPurge() <-chan struct{} {
	s.purgeMu.Lock()
	defer s.purgeMu.Unlock()
	s.purgeStopped = true
	if !s.purgeStarted {
		return nil
	}
	s.purgeCancel()
	return s.purgeDone
}

// awaitPurge waits, within ctx, for done from [Storage.stopPurge].
func (s *Storage) awaitPurge(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	// A loop that already exited satisfies even an ended ctx.
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return coreerrs.Wrap(ctx.Err(), "wait for audit retention to stop")
	}
}

func (s *Storage) purgeLoop(ctx context.Context) {
	defer close(s.purgeDone)
	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false))
	ticker := time.NewTicker(s.opts.purgeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runPurge(ctx)
		}
	}
}

// Recover each pass separately so a panic does not terminate retention.
func (s *Storage) runPurge(ctx context.Context) {
	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(ctx context.Context, _ any) {
		s.opts.logger.ErrorContext(ctx, "audit retention pass panicked")
	})
	if _, err := s.PurgeExpired(ctx); err != nil && ctx.Err() == nil {
		s.opts.logger.ErrorContext(ctx, "purge expired audit events failed", "error", err)
	}
}

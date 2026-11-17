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

// Close permanently stops background purging and waits for it within ctx.
// Explicit PurgeExpired calls remain available. It does not close the database.
func (s *Store[P]) Close(ctx context.Context) error {
	return s.awaitPurge(ctx, s.stopPurge())
}

// startPurge starts the background retention loop once when purging is enabled, unless
// the storage was closed. The loop keeps ctx's values but not its
// cancellation or business transaction: it runs until Close.
func (s *Store[P]) startPurge(ctx context.Context) {
	if s.retention <= 0 || s.purgeEvery <= 0 {
		return
	}
	s.purgeMu.Lock()
	defer s.purgeMu.Unlock()
	if s.purgeStarted || s.purgeStopped {
		return
	}
	s.purgeStarted = true
	ctx, cancel := context.WithCancel(WithTx(context.WithoutCancel(ctx), nil))
	s.purgeCancel, s.purgeDone = cancel, make(chan struct{})
	go s.purgeLoop(ctx)
}

// stopPurge stops the background retention loop for good and returns a
// channel closed once it exited, or nil when it never started.
func (s *Store[P]) stopPurge() <-chan struct{} {
	s.purgeMu.Lock()
	defer s.purgeMu.Unlock()
	s.purgeStopped = true
	if !s.purgeStarted {
		return nil
	}
	s.purgeCancel()
	return s.purgeDone
}

// awaitPurge waits, within ctx, for done from [Store.stopPurge].
func (s *Store[P]) awaitPurge(ctx context.Context, done <-chan struct{}) error {
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
		return coreerrs.Wrap(ctx.Err(), "wait for SQL purge to stop")
	}
}

func (s *Store[P]) purgeLoop(ctx context.Context) {
	defer close(s.purgeDone)
	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false))
	ticker := time.NewTicker(s.purgeEvery)
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
func (s *Store[P]) runPurge(ctx context.Context) {
	defer panics.HandleWithOpts(ctx, panics.NewHandleOpts().SetReallyPanic(false), func(ctx context.Context, _ any) {
		s.logger.ErrorContext(ctx, "resolved retry item purge panicked")
	})
	if _, err := s.PurgeExpired(ctx); err != nil && ctx.Err() == nil {
		s.logger.ErrorContext(ctx, "failed to purge resolved retry items", "error", err)
	}
}

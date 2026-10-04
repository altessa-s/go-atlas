// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch

import (
	"fmt"
	"time"

	"github.com/altessa-s/go-atlas/core/io/wal"
)

func (e *Engine[T]) notifyWorker() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// claimBatch serializes selection, not delivery. The in-flight set is bounded
// by workers * batchSize; backlog payloads remain in the journal.
func (e *Engine[T]) claimBatch() ([]wal.Record, error) {
	e.claimMu.Lock()
	defer e.claimMu.Unlock()
	records, err := e.log.ReadPending(e.opts.batchSize, func(off wal.Offset) bool {
		_, ok := e.inFlight[off]
		return ok
	})
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		e.inFlight[record.Offset] = struct{}{}
	}
	return records, nil
}

func (e *Engine[T]) durableWorker() {
	e.metrics.workersActive.Inc()
	defer e.metrics.workersActive.Dec()
	ticker := time.NewTicker(e.opts.flushInterval)
	defer ticker.Stop()
	for {
		records, err := e.claimBatch()
		if err != nil {
			e.lastError.Store(&err)
			return
		}
		if len(records) == 0 {
			select {
			case <-e.drainDone:
				return
			case <-e.wake:
			case <-ticker.C:
			}
			continue
		}
		items := make([]T, 0, len(records))
		offsets := make([]wal.Offset, 0, len(records))
		for _, record := range records {
			item, err := e.codec.Decode(record.Payload)
			if err != nil {
				err = fmt.Errorf("%w: %w", ErrReplayDecode, err)
				e.lastError.Store(&err)
				return // Preserve the record; codec errors never acknowledge data.
			}
			items = append(items, item)
			offsets = append(offsets, record.Offset)
		}
		if err := e.storeBatch(items, offsets); err != nil {
			e.lastError.Store(&err)
			return
		}
		e.claimMu.Lock()
		for _, off := range offsets {
			delete(e.inFlight, off)
		}
		e.claimMu.Unlock()
		e.observeWALSize()
		e.notifyWorker()
	}
}

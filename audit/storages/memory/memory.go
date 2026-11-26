// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"iter"
	"slices"
	"sync"

	"github.com/altessa-s/go-atlas/audit"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

var _ audit.Storage = (*Storage)(nil)

const (
	initialRingAlloc = 16
	ringGrowthFactor = 2
)

// Storage is a bounded in-memory audit event store backed by a ring buffer.
// When capacity is reached, storing new events evicts the oldest stored events.
type Storage struct {
	mu        sync.RWMutex
	buf       []*audit.Event
	head      int
	count     int
	maxEvents int
}

// New creates a new bounded in-memory [Storage].
func New(opts ...Option) *Storage {
	o := newOptions(opts...)
	return &Storage{
		maxEvents: o.maxEvents,
	}
}

// Store appends a single event to the ring buffer, evicting the oldest event when full.
func (s *Storage) Store(_ context.Context, event *audit.Event) error {
	if event == nil {
		return coreerrs.Wrap(coreerrs.ErrInvalidArgument, "nil audit event")
	}
	s.mu.Lock()
	s.pushLocked(event)
	s.mu.Unlock()
	return nil
}

// StoreBatch appends all events to the ring buffer in a single lock acquisition,
// evicting the oldest events when capacity is exceeded. A nil element rejects the
// entire batch before any write.
func (s *Storage) StoreBatch(_ context.Context, events []*audit.Event) error {
	if slices.Contains(events, nil) {
		return coreerrs.Wrap(coreerrs.ErrInvalidArgument, "nil audit event")
	}
	if len(events) == 0 {
		return nil
	}
	s.mu.Lock()
	if len(events) >= s.maxEvents {
		if len(s.buf) < s.maxEvents {
			s.buf = make([]*audit.Event, s.maxEvents)
		}
		copy(s.buf, events[len(events)-s.maxEvents:])
		s.head = 0
		s.count = s.maxEvents
	} else {
		for _, e := range events {
			s.pushLocked(e)
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *Storage) pushLocked(event *audit.Event) {
	if s.count < len(s.buf) {
		idx := s.head + s.count
		if idx >= len(s.buf) {
			idx -= len(s.buf)
		}
		s.buf[idx] = event
		s.count++
		return
	}
	if len(s.buf) < s.maxEvents {
		newCap := min(max(len(s.buf)*ringGrowthFactor, initialRingAlloc), s.maxEvents)
		next := make([]*audit.Event, newCap)
		s.copyIntoLocked(next)
		next[s.count] = event
		s.buf = next
		s.head = 0
		s.count++
		return
	}
	s.buf[s.head] = event
	s.head++
	if s.head == s.maxEvents {
		s.head = 0
	}
}

func (s *Storage) copyIntoLocked(dst []*audit.Event) {
	if s.count == 0 {
		return
	}
	first := min(s.count, len(s.buf)-s.head)
	copy(dst, s.buf[s.head:s.head+first])
	if first < s.count {
		copy(dst[first:], s.buf[:s.count-first])
	}
}

func (s *Storage) snapshotLocked() []*audit.Event {
	if s.count == 0 {
		return nil
	}
	out := make([]*audit.Event, s.count)
	s.copyIntoLocked(out)
	return out
}

// Query returns events matching the query in the total order of the
// timestamp (milliseconds) and ID, newest first unless ascending order is
// requested, starting after query.Cursor. It sorts a snapshot taken under a
// read lock and iterates without holding the lock.
func (s *Storage) Query(_ context.Context, query *audit.Query) iter.Seq2[*audit.Event, error] {
	if query == nil {
		query = &audit.Query{}
	}
	return func(yield func(*audit.Event, error) bool) {
		s.mu.RLock()
		snapshot := s.snapshotLocked()
		s.mu.RUnlock()

		asc := query.SortOrder == audit.SortOrderAsc
		slices.SortStableFunc(snapshot, func(a, b *audit.Event) int {
			c := audit.CursorOf(a).Compare(audit.CursorOf(b))
			if asc {
				return c
			}
			return -c
		})

		limit := query.Limit
		if limit <= 0 {
			limit = len(snapshot)
		}

		count := 0
		for _, e := range snapshot {
			if !matchesQuery(e, query) {
				continue
			}
			if query.Cursor != nil && !query.Cursor.Follows(e, query.SortOrder) {
				continue
			}
			if count >= limit {
				return
			}
			if !yield(e, nil) {
				return
			}
			count++
		}
	}
}

// Count returns the number of events matching the query filter. The page
// position, size and order are ignored.
func (s *Storage) Count(_ context.Context, query *audit.Query) (int64, error) {
	if query == nil {
		query = &audit.Query{}
	}
	s.mu.RLock()
	snapshot := s.snapshotLocked()
	s.mu.RUnlock()

	var count int64
	for _, e := range snapshot {
		if matchesQuery(e, query) {
			count++
		}
	}
	return count, nil
}

// Close is a no-op for in-memory storage.
func (s *Storage) Close(_ context.Context) error {
	return nil
}

// Events returns all stored events from oldest to newest (for test assertions).
func (s *Storage) Events() []*audit.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

// Len returns the number of stored events.
func (s *Storage) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}

// Cap returns the maximum number of events retained by the ring buffer.
func (s *Storage) Cap() int {
	return s.maxEvents
}

// Reset clears all stored events and releases their backing references.
func (s *Storage) Reset() {
	s.mu.Lock()
	clear(s.buf)
	s.head = 0
	s.count = 0
	s.mu.Unlock()
}

// matchesQuery reports whether e satisfies the query filter. The time range
// is compared at millisecond precision, as the persistent storages store and
// bound timestamps, so a bound with a sub-millisecond part selects the same
// events everywhere.
func matchesQuery(e *audit.Event, q *audit.Query) bool {
	ms := e.Timestamp.UnixMilli()
	if q.StartTime != nil && ms < q.StartTime.UnixMilli() {
		return false
	}
	if q.EndTime != nil && ms > q.EndTime.UnixMilli() {
		return false
	}
	if q.ActorID != "" && e.Actor.ID != q.ActorID {
		return false
	}
	if q.ActorType != "" && string(e.Actor.Type) != q.ActorType {
		return false
	}
	if q.ResourceType != "" && e.Resource.Type != q.ResourceType {
		return false
	}
	if q.ResourceID != "" && e.Resource.ID != q.ResourceID {
		return false
	}
	if q.EventType != "" && e.Type != q.EventType {
		return false
	}
	if q.Action != "" && e.Action != q.Action {
		return false
	}
	if q.Status != "" && e.Result.Status != q.Status {
		return false
	}
	if q.RequestID != "" && e.Context.RequestID != q.RequestID {
		return false
	}
	if q.TraceID != "" && e.Context.TraceID != q.TraceID {
		return false
	}
	return true
}

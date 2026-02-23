// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-memory audit storage implementation for testing.
package memory

import (
	"context"
	"iter"
	"slices"
	"sync"

	"github.com/altessa-s/go-atlas/data/audit"
)

var _ audit.Storage = (*Storage)(nil)

// Storage is an in-memory audit event store for testing purposes.
type Storage struct {
	mu     sync.RWMutex
	events []*audit.Event
}

// New creates a new in-memory Storage.
func New() *Storage {
	return &Storage{}
}

// Store appends a single event to the in-memory slice.
func (s *Storage) Store(_ context.Context, event *audit.Event) error {
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	return nil
}

// StoreBatch appends all events to the in-memory slice in a single lock acquisition.
func (s *Storage) StoreBatch(_ context.Context, events []*audit.Event) error {
	s.mu.Lock()
	s.events = append(s.events, events...)
	s.mu.Unlock()
	return nil
}

// Query returns events matching the query criteria in reverse chronological order.
// It snapshots the current events under a read lock and iterates without holding the lock.
func (s *Storage) Query(_ context.Context, query *audit.Query) iter.Seq2[*audit.Event, error] {
	return func(yield func(*audit.Event, error) bool) {
		s.mu.RLock()
		// Copy to release lock quickly.
		snapshot := slices.Clone(s.events)
		s.mu.RUnlock()

		offset := query.Offset
		limit := query.Limit
		if limit <= 0 {
			limit = len(snapshot)
		}

		count := 0
		for i := len(snapshot) - 1; i >= 0; i-- {
			e := snapshot[i]
			if !matchesQuery(e, query) {
				continue
			}
			if offset > 0 {
				offset--
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

// Count returns the number of events matching the query criteria.
func (s *Storage) Count(_ context.Context, query *audit.Query) (int64, error) {
	s.mu.RLock()
	snapshot := slices.Clone(s.events)
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

// Events returns all stored events (for test assertions).
func (s *Storage) Events() []*audit.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.events)
}

// Len returns the number of stored events.
func (s *Storage) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

// Reset clears all stored events.
func (s *Storage) Reset() {
	s.mu.Lock()
	s.events = s.events[:0]
	s.mu.Unlock()
}

func matchesQuery(e *audit.Event, q *audit.Query) bool {
	if q.StartTime != nil && e.Timestamp.Before(*q.StartTime) {
		return false
	}
	if q.EndTime != nil && e.Timestamp.After(*q.EndTime) {
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

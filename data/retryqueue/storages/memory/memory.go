// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/data/retryqueue"
)

var _ retryqueue.Storage[struct{}] = (*Store[struct{}])(nil)

// Store is a map-backed [retryqueue.Storage]. Every method runs under one
// mutex, so each transition is atomic. Items are stored and returned by value;
// a payload holding references (slices, maps, pointers) shares them with the
// caller.
type Store[P any] struct {
	mu    sync.Mutex
	items map[string]retryqueue.Item[P]
}

// New returns an empty Store.
func New[P any]() *Store[P] {
	return &Store[P]{items: map[string]retryqueue.Item[P]{}}
}

// Len returns the number of stored items in any state.
func (s *Store[P]) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

// at returns the item of key when it is at generation gen. The caller holds mu.
func (s *Store[P]) at(key string, gen int64) (retryqueue.Item[P], bool) {
	it, ok := s.items[key]
	return it, ok && it.Generation == gen
}

// fresh returns item as a new pending item, as Mark inserts it.
func fresh[P any](item retryqueue.Item[P]) retryqueue.Item[P] {
	return retryqueue.Item[P]{
		Key:           item.Key,
		Generation:    item.Generation,
		State:         retryqueue.StatePending,
		Since:         utc(item.Since),
		NextAttemptAt: utc(item.NextAttemptAt),
		Payload:       item.Payload,
	}
}

// utc normalizes t to UTC, keeping the zero time zero.
func utc(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC()
}

// Mark implements [retryqueue.Storage].
func (s *Store[P]) Mark(_ context.Context, item retryqueue.Item[P]) (retryqueue.Item[P], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.items[item.Key]; ok {
		return cur, nil
	}
	it := fresh(item)
	s.items[item.Key] = it
	return it, nil
}

// Reopen implements [retryqueue.Storage].
func (s *Store[P]) Reopen(_ context.Context, item retryqueue.Item[P]) (retryqueue.Item[P], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.items[item.Key]
	if ok && (cur.Generation > item.Generation ||
		(cur.Generation == item.Generation && cur.State == retryqueue.StatePending)) {
		return cur, nil
	}
	it := fresh(item)
	s.items[item.Key] = it
	return it, nil
}

// ListDue implements [retryqueue.Storage].
func (s *Store[P]) ListDue(_ context.Context, now time.Time, limit int) ([]retryqueue.Item[P], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.collect(func(it retryqueue.Item[P]) bool {
		return it.State == retryqueue.StatePending && !it.NextAttemptAt.After(now)
	})
	slices.SortFunc(out, func(a, b retryqueue.Item[P]) int {
		return cmp.Or(a.NextAttemptAt.Compare(b.NextAttemptAt), cmp.Compare(a.Key, b.Key))
	})
	return truncate(out, limit), nil
}

// WakeDue implements [retryqueue.Storage].
func (s *Store[P]) WakeDue(_ context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, it := range s.items {
		if it.State == retryqueue.StatePending && it.NextAttemptAt.After(now) {
			it.NextAttemptAt = now.UTC()
			s.items[key] = it
		}
	}
	return nil
}

// Reschedule implements [retryqueue.Storage].
func (s *Store[P]) Reschedule(_ context.Context, key string, generation int64, next time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it, ok := s.at(key, generation); ok && it.State == retryqueue.StatePending {
		it.Attempts++
		it.NextAttemptAt = next.UTC()
		s.items[key] = it
	}
	return nil
}

// Expire implements [retryqueue.Storage].
func (s *Store[P]) Expire(_ context.Context, key string, generation int64, since, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.at(key, generation)
	if !ok || it.State != retryqueue.StatePending || !it.Since.Equal(since) {
		return false, nil
	}
	it.State = retryqueue.StateExpired
	it.ExpiredAt = at.UTC()
	s.items[key] = it
	return true, nil
}

// Resolve implements [retryqueue.Storage].
func (s *Store[P]) Resolve(_ context.Context, key string, generation int64, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it, ok := s.at(key, generation); ok && it.State != retryqueue.StateResolved {
		it.State = retryqueue.StateResolved
		it.ResolvedAt = at.UTC()
		it.ExpiredAt = time.Time{}
		it.Flagged, it.FlagReason = false, ""
		s.items[key] = it
	}
	return nil
}

// Flag implements [retryqueue.Storage].
func (s *Store[P]) Flag(_ context.Context, key string, generation int64, reason string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.at(key, generation)
	if !ok || it.State != retryqueue.StatePending || (it.Flagged && it.FlagReason == reason) {
		return false, nil
	}
	it.Flagged, it.FlagReason = true, reason
	s.items[key] = it
	return true, nil
}

// ClearFlag implements [retryqueue.Storage].
func (s *Store[P]) ClearFlag(_ context.Context, key string, generation int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it, ok := s.at(key, generation); ok && it.Flagged {
		it.Flagged, it.FlagReason = false, ""
		s.items[key] = it
	}
	return nil
}

// DeleteObsolete implements [retryqueue.Storage].
func (s *Store[P]) DeleteObsolete(_ context.Context, key string, generation int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[key]
	if !ok {
		return false, nil
	}
	if it.Generation > generation {
		return true, nil
	}
	delete(s.items, key)
	return false, nil
}

// Delete implements [retryqueue.Storage].
func (s *Store[P]) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}

// ListByState implements [retryqueue.Storage].
func (s *Store[P]) ListByState(_ context.Context, state retryqueue.State, limit int) ([]retryqueue.Item[P], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.collect(func(it retryqueue.Item[P]) bool { return it.State == state })
	slices.SortFunc(out, func(a, b retryqueue.Item[P]) int {
		return cmp.Or(a.Since.Compare(b.Since), cmp.Compare(a.Key, b.Key))
	})
	return truncate(out, limit), nil
}

// Requeue implements [retryqueue.Storage].
func (s *Store[P]) Requeue(_ context.Context, now time.Time, keys ...string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC()
	moved := 0
	for key, it := range s.items {
		if it.State != retryqueue.StateExpired || (len(keys) > 0 && !slices.Contains(keys, key)) {
			continue
		}
		it.State = retryqueue.StatePending
		it.Since, it.NextAttemptAt = now, now
		it.Attempts = 0
		it.ExpiredAt = time.Time{}
		s.items[key] = it
		moved++
	}
	return moved, nil
}

// collect returns the items keep accepts. The caller holds mu.
func (s *Store[P]) collect(keep func(retryqueue.Item[P]) bool) []retryqueue.Item[P] {
	var out []retryqueue.Item[P]
	for _, it := range s.items {
		if keep(it) {
			out = append(out, it)
		}
	}
	return out
}

// truncate caps items at limit, or at [retryqueue.DefaultListLimit] when limit
// is not positive. It never returns nil.
func truncate[P any](items []retryqueue.Item[P], limit int) []retryqueue.Item[P] {
	if limit <= 0 {
		limit = retryqueue.DefaultListLimit
	}
	if len(items) > limit {
		items = items[:limit]
	}
	if items == nil {
		return []retryqueue.Item[P]{}
	}
	return items
}

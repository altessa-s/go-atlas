// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/reliability/retryqueue"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

var (
	_ retryqueue.Storage[struct{}] = (*Store[struct{}])(nil)
	_ retryqueue.Mover             = (*Store[struct{}])(nil)
)

// Store is a map-backed [retryqueue.Storage]. Every method runs under one
// mutex, so each transition is atomic, and checks its context before waiting
// for the mutex and again once it holds it: a call whose context is done
// fails with the context error and changes nothing. Items are stored and
// returned by value; a payload holding references (slices, maps, pointers)
// shares them with the caller.
type Store[P any] struct {
	mu    sync.Mutex
	items map[string]retryqueue.Item[P]
	// beforeLock, when set, runs in lock after the first context check and
	// before waiting for mu. It is a test seam, nil in production: a test
	// holding mu learns that a call passed the check and is about to block.
	beforeLock func()
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

// lock acquires mu for a call under ctx. It fails with the context error,
// wrapped with op as the MongoDB backend wraps a driver error, when ctx is
// done before the wait or when it ended while the call waited for mu (mu is
// then released): a call whose caller gave up never changes the store.
func (s *Store[P]) lock(ctx context.Context, op string) error {
	if err := ctx.Err(); err != nil {
		return coreerrs.WrapOperation(err, op)
	}
	if s.beforeLock != nil {
		s.beforeLock()
	}
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return coreerrs.WrapOperation(err, op)
	}
	return nil
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
func (s *Store[P]) Mark(ctx context.Context, item retryqueue.Item[P]) (retryqueue.Item[P], error) {
	if err := s.lock(ctx, "mark retry item"); err != nil {
		return retryqueue.Item[P]{}, err
	}
	defer s.mu.Unlock()
	if cur, ok := s.items[item.Key]; ok {
		return cur, nil
	}
	it := fresh(item)
	s.items[item.Key] = it
	return it, nil
}

// Reopen implements [retryqueue.Storage].
func (s *Store[P]) Reopen(ctx context.Context, item retryqueue.Item[P]) (retryqueue.Item[P], error) {
	if err := s.lock(ctx, "reopen retry item"); err != nil {
		return retryqueue.Item[P]{}, err
	}
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
func (s *Store[P]) ListDue(ctx context.Context, now time.Time, limit int) ([]retryqueue.Item[P], error) {
	if err := s.lock(ctx, "list due retry items"); err != nil {
		return nil, err
	}
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
func (s *Store[P]) WakeDue(ctx context.Context, now time.Time) error {
	if err := s.lock(ctx, "wake due retry items"); err != nil {
		return err
	}
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
func (s *Store[P]) Reschedule(ctx context.Context, key string, generation int64, next time.Time) error {
	if err := s.lock(ctx, "reschedule retry item"); err != nil {
		return err
	}
	defer s.mu.Unlock()
	if it, ok := s.at(key, generation); ok && it.State == retryqueue.StatePending {
		it.Attempts++
		it.NextAttemptAt = next.UTC()
		s.items[key] = it
	}
	return nil
}

// Expire implements [retryqueue.Storage].
func (s *Store[P]) Expire(ctx context.Context, key string, generation int64, since, at time.Time) (bool, error) {
	if err := s.lock(ctx, "expire retry item"); err != nil {
		return false, err
	}
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
func (s *Store[P]) Resolve(ctx context.Context, key string, generation int64, at time.Time) error {
	if err := s.lock(ctx, "resolve retry item"); err != nil {
		return err
	}
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
func (s *Store[P]) Flag(ctx context.Context, key string, generation int64, reason string) (bool, error) {
	if err := s.lock(ctx, "flag retry item"); err != nil {
		return false, err
	}
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
func (s *Store[P]) ClearFlag(ctx context.Context, key string, generation int64) error {
	if err := s.lock(ctx, "clear retry item flag"); err != nil {
		return err
	}
	defer s.mu.Unlock()
	if it, ok := s.at(key, generation); ok && it.Flagged {
		it.Flagged, it.FlagReason = false, ""
		s.items[key] = it
	}
	return nil
}

// DeleteObsolete implements [retryqueue.Storage].
func (s *Store[P]) DeleteObsolete(ctx context.Context, key string, generation int64) (bool, error) {
	if err := s.lock(ctx, "delete obsolete retry item"); err != nil {
		return false, err
	}
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
func (s *Store[P]) Delete(ctx context.Context, key string) error {
	if err := s.lock(ctx, "delete retry item"); err != nil {
		return err
	}
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}

// ListByState implements [retryqueue.Storage].
func (s *Store[P]) ListByState(ctx context.Context, state retryqueue.State, limit int) ([]retryqueue.Item[P], error) {
	if err := s.lock(ctx, "list retry items by state"); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	out := s.collect(func(it retryqueue.Item[P]) bool { return it.State == state })
	slices.SortFunc(out, func(a, b retryqueue.Item[P]) int {
		return cmp.Or(a.Since.Compare(b.Since), cmp.Compare(a.Key, b.Key))
	})
	return truncate(out, limit), nil
}

// Requeue implements [retryqueue.Storage].
func (s *Store[P]) Requeue(ctx context.Context, now time.Time, keys ...string) (int, error) {
	if err := s.lock(ctx, "requeue retry items"); err != nil {
		return 0, err
	}
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

// Move implements [retryqueue.Mover] under the store's mutex: the item is
// copied whole to the new key and its old key removed in one critical
// section.
func (s *Store[P]) Move(ctx context.Context, from, to string) error {
	if err := s.lock(ctx, "move retry item"); err != nil {
		return err
	}
	defer s.mu.Unlock()
	it, ok := s.items[from]
	switch {
	case !ok:
		return fmt.Errorf("move %q: %w", from, retryqueue.ErrItemNotFound)
	case from == to:
		return nil
	}
	if _, taken := s.items[to]; taken {
		return fmt.Errorf("move %q to %q: %w", from, to, retryqueue.ErrItemExists)
	}
	it.Key = to
	s.items[to] = it
	delete(s.items, from)
	return nil
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

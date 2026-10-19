// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"sync"

	"github.com/altessa-s/go-atlas/core/time/clock"
	"github.com/altessa-s/go-atlas/data/cache/generation"
)

// Store is an in-process [generation.Store]. A bump is visible to every later
// Current call at once; it is not shared with other processes. It never
// fails.
type Store struct {
	clock clock.Clock

	mu   sync.Mutex
	gens map[string]uint64
}

var _ generation.Store = (*Store)(nil)

// New returns an empty in-process store. A namespace is seeded with
// [generation.Seed] of the configured clock when it is first used.
func New(opts ...Option) *Store {
	o := newOptions(opts...)
	return &Store{clock: o.clock, gens: make(map[string]uint64)}
}

// Current implements [generation.Store].
func (s *Store) Current(_ context.Context, namespace string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentLocked(namespace), nil
}

// Bump implements [generation.Store]. The new generation is the larger of the
// previous one plus one and the seed of the current time, so it keeps growing
// even when the clock is behind the stored value.
func (s *Store) Bump(_ context.Context, namespace string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := max(s.currentLocked(namespace)+1, generation.Seed(s.clock.Now()))
	s.gens[namespace] = next
	return next, nil
}

// currentLocked returns the generation of namespace, seeding it on first use.
func (s *Store) currentLocked(namespace string) uint64 {
	gen, ok := s.gens[namespace]
	if !ok {
		gen = generation.Seed(s.clock.Now())
		s.gens[namespace] = gen
	}
	return gen
}
